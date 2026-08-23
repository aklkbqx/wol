package localremote

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	mu       sync.Mutex
	pathErr  error
	commands []Command
	run      func(Command) (Result, error)
}

func (f *fakeRunner) LookPath(string) (string, error) {
	if f.pathErr != nil {
		return "", f.pathErr
	}
	return "/usr/bin/docker", nil
}

func (f *fakeRunner) Run(_ context.Context, command Command) (Result, error) {
	f.mu.Lock()
	f.commands = append(f.commands, command)
	f.mu.Unlock()
	if f.run != nil {
		return f.run(command)
	}
	return Result{}, nil
}

func (f *fakeRunner) snapshot() []Command {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Command(nil), f.commands...)
}

func TestDoctorReportsMissingDocker(t *testing.T) {
	runner := &fakeRunner{pathErr: errors.New("missing")}
	report, err := Doctor(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready() || report.DockerCLI || len(report.Problems) != 1 || !strings.Contains(report.Problems[0], "not installed") {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestDoctorReportsDaemonAndImages(t *testing.T) {
	runner := &fakeRunner{run: func(command Command) (Result, error) {
		if reflect.DeepEqual(command.Args[:min(2, len(command.Args))], []string{"image", "inspect"}) && command.Args[2] == GuacdImage {
			return Result{Stderr: "No such image"}, errors.New("exit 1")
		}
		return Result{}, nil
	}}
	report, err := Doctor(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready() || !report.DockerCLI || !report.DockerDaemon || !report.Images[GuacamoleImage] || report.Images[GuacdImage] {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestSetupPullsOnlyPinnedImagesWithoutShell(t *testing.T) {
	runner := &fakeRunner{}
	if err := Setup(context.Background(), runner); err != nil {
		t.Fatal(err)
	}
	commands := runner.snapshot()
	want := [][]string{
		{"version", "--format", "{{.Server.Version}}"},
		{"ps", "--all", "--quiet", "--filter", "label=" + ownershipLabel + "=true"},
		{"network", "ls", "--quiet", "--filter", "label=" + ownershipLabel + "=true"},
		{"pull", GuacdImage},
		{"pull", GuacamoleImage},
	}
	if len(commands) != len(want) {
		t.Fatalf("commands = %#v", commands)
	}
	for i, command := range commands {
		if command.Name != "docker" || !reflect.DeepEqual(command.Args, want[i]) || strings.Contains(strings.Join(command.Args, " "), "sh -c") {
			t.Fatalf("command %d = %#v", i, command)
		}
	}
}

func TestSetupSurfacesDaemonAndPullErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		fail string
		want string
	}{
		{"daemon", "version", "daemon is unavailable"},
		{"pull", "pull", "pull " + GuacdImage},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeRunner{run: func(command Command) (Result, error) {
				if len(command.Args) > 0 && command.Args[0] == test.fail {
					return Result{Stderr: "detail"}, errors.New("exit 1")
				}
				return Result{}, nil
			}}
			err := Setup(context.Background(), runner)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "detail") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestCleanupOrphansRemovesOnlyDeadOwners(t *testing.T) {
	runner := &fakeRunner{run: func(command Command) (Result, error) {
		switch {
		case command.Args[0] == "ps":
			return Result{Stdout: "dead-container\n"}, nil
		case command.Args[0] == "inspect":
			return Result{Stdout: "99999999\n"}, nil
		case command.Args[0] == "network" && command.Args[1] == "ls":
			return Result{Stdout: "dead-network\n"}, nil
		case command.Args[0] == "network" && command.Args[1] == "inspect":
			return Result{Stdout: "99999999\n"}, nil
		default:
			return Result{}, nil
		}
	}}
	if err := cleanupOrphans(t.Context(), runner); err != nil {
		t.Fatal(err)
	}
	commands := runner.snapshot()
	var removedContainer, removedNetwork bool
	for _, command := range commands {
		removedContainer = removedContainer || reflect.DeepEqual(command.Args, []string{"rm", "--force", "dead-container"})
		removedNetwork = removedNetwork || reflect.DeepEqual(command.Args, []string{"network", "rm", "dead-network"})
	}
	if !removedContainer || !removedNetwork {
		t.Fatalf("orphan cleanup commands = %#v", commands)
	}
}

func TestBuildAuthTokenMatchesGuacamoleJSONAuthFormat(t *testing.T) {
	key := []byte("0123456789abcdef")
	token, err := buildAuthToken(key, Config{Protocol: "rdp", Host: "192.168.50.200", Port: 3389, UsernameHint: "desktop-user"}, time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(key)
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(plain, ciphertext)
	pad := int(plain[len(plain)-1])
	plain = plain[:len(plain)-pad]
	signature, document := plain[:sha256.Size], plain[sha256.Size:]
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(document)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		t.Fatal("invalid HMAC")
	}
	var got authDocument
	if err := json.Unmarshal(document, &got); err != nil {
		t.Fatal(err)
	}
	connection := got.Connections["Remote session"]
	if got.Expires != 1_700_000_120_000 || connection.Protocol != "rdp" || connection.Parameters["hostname"] != "192.168.50.200" || connection.Parameters["username"] != "desktop-user" {
		t.Fatalf("document = %#v", got)
	}
	if _, found := connection.Parameters["password"]; found {
		t.Fatal("launch token must not contain a password")
	}
}

func TestBrokerOneTimeTokenHostOriginCookieAndPage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("upstream " + r.URL.Path))
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)

	server := httptest.NewUnstartedServer(nil)
	server.Listener.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	host := listener.Addr().String()
	server.Config.Handler = newBroker(host, "once", "cookie", "encrypted", upstreamURL)
	server.Start()
	defer server.Close()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/s/once", nil)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/session" {
		t.Fatalf("consume status = %d", response.StatusCode)
	}
	cookie := response.Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("weak session cookie: %#v", cookie)
	}
	_ = response.Body.Close()

	response, _ = client.Get(server.URL + "/s/once")
	if response.StatusCode != http.StatusGone {
		t.Fatalf("reused token status = %d", response.StatusCode)
	}
	_ = response.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/session", nil)
	req.AddCookie(cookie)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "LOCAL REMOTE") || !strings.Contains(string(body), "/guacamole/?data=encrypted") {
		t.Fatalf("page status/body = %d %q", response.StatusCode, body)
	}

	for _, mutate := range []func(*http.Request){
		func(r *http.Request) { r.Host = "evil.test" },
		func(r *http.Request) { r.Header.Set("Origin", "https://evil.test") },
	} {
		req, _ = http.NewRequest(http.MethodGet, server.URL+"/session", nil)
		req.AddCookie(cookie)
		mutate(req)
		response, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("guard status = %d", response.StatusCode)
		}
		_ = response.Body.Close()
	}

	response, _ = client.Get(server.URL + "/session")
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing cookie status = %d", response.StatusCode)
	}
	_ = response.Body.Close()
}

func TestStartBindsLoopbackBuildsCommandsOpensAndCleansUp(t *testing.T) {
	upstream := loopbackServer(t)
	defer upstream.Close()
	upstreamHost := strings.TrimPrefix(upstream.URL, "http://")
	runner := dockerFake(upstreamHost)
	var opened string
	session, err := Start(context.Background(), Config{
		Protocol: "rdp", Host: "192.168.50.200", Port: 3389,
		Runner: runner, OpenBrowser: true,
		Opener: func(_ context.Context, target string) error { opened = target; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(session.URL)
	if parsed.Hostname() != "127.0.0.1" || opened != session.URL || !strings.HasPrefix(parsed.Path, "/s/") {
		t.Fatalf("session URL/opened = %q %q", session.URL, opened)
	}
	commands := runner.snapshot()
	if len(commands) != 6 {
		t.Fatalf("start commands = %#v", commands)
	}
	if !reflect.DeepEqual(commands[2].Args[:3], []string{"network", "create", "--driver"}) {
		t.Fatalf("network command = %#v", commands[2])
	}
	guac := commands[4]
	joined := strings.Join(guac.Args, " ")
	if !strings.Contains(joined, "127.0.0.1::8080") || !strings.Contains(joined, GuacamoleImage) || strings.Contains(joined, "JSON_SECRET_KEY=") || len(guac.Env) != 1 {
		t.Fatalf("Guacamole command leaks or is malformed: %#v", guac)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	commands = runner.snapshot()
	if len(commands) != 9 || !reflect.DeepEqual(commands[6].Args[:2], []string{"rm", "--force"}) || commands[8].Args[0] != "network" {
		t.Fatalf("cleanup commands = %#v", commands[6:])
	}
}

func TestStartFailureCleansCreatedResources(t *testing.T) {
	runner := &fakeRunner{run: func(command Command) (Result, error) {
		if len(command.Args) > 0 && command.Args[0] == "run" {
			return Result{Stderr: "boom"}, errors.New("exit 1")
		}
		return Result{}, nil
	}}
	_, err := Start(context.Background(), Config{Protocol: "vnc", Host: "machine.lan", Port: 5900, Runner: runner})
	if err == nil || !strings.Contains(err.Error(), "start guacd") {
		t.Fatalf("error = %v", err)
	}
	commands := runner.snapshot()
	last := commands[len(commands)-1]
	if !reflect.DeepEqual(last.Args[:2], []string{"network", "rm"}) {
		t.Fatalf("last cleanup command = %#v", last)
	}
}

func TestValidateConfig(t *testing.T) {
	valid := Config{Protocol: "ssh", Host: "host.lan", Port: 22, UsernameHint: "user"}
	if err := validateConfig(valid); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []Config{
		{Protocol: "http", Host: "host", Port: 80},
		{Protocol: "rdp", Host: "", Port: 3389},
		{Protocol: "rdp", Host: "evil/path", Port: 3389},
		{Protocol: "rdp", Host: "host", Port: 0},
		{Protocol: "rdp", Host: "host", Port: 3389, UsernameHint: "bad\nuser"},
	} {
		if err := validateConfig(cfg); err == nil {
			t.Fatalf("accepted invalid config: %#v", cfg)
		}
	}
}

func TestParsePublishedLoopbackRejectsNonLoopback(t *testing.T) {
	if got, err := parsePublishedLoopback("127.0.0.1:49152\n"); err != nil || got != "http://127.0.0.1:49152" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, value := range []string{"0.0.0.0:49152", "[::]:49152", "garbage"} {
		if _, err := parsePublishedLoopback(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

func loopbackServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	server.Listener.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	server.Start()
	return server
}

func dockerFake(upstreamHost string) *fakeRunner {
	return &fakeRunner{run: func(command Command) (Result, error) {
		if len(command.Args) > 0 && (command.Args[0] == "ps" || (len(command.Args) > 1 && command.Args[0] == "network" && command.Args[1] == "ls")) {
			return Result{}, nil
		}
		if len(command.Args) > 0 && command.Args[0] == "port" {
			return Result{Stdout: upstreamHost + "\n"}, nil
		}
		return Result{Stdout: "ok"}, nil
	}}
}

func TestContextCancellationCleansUp(t *testing.T) {
	upstream := loopbackServer(t)
	defer upstream.Close()
	runner := dockerFake(strings.TrimPrefix(upstream.URL, "http://"))
	ctx, cancel := context.WithCancel(context.Background())
	session, err := Start(ctx, Config{Protocol: "rdp", Host: "host.lan", Port: 3389, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for len(runner.snapshot()) < 9 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(runner.snapshot()) != 9 {
		t.Fatalf("context cancellation did not clean up: %#v", runner.snapshot())
	}
	_ = session.Close()
}

func TestOpenFailureCleansUp(t *testing.T) {
	upstream := loopbackServer(t)
	defer upstream.Close()
	runner := dockerFake(strings.TrimPrefix(upstream.URL, "http://"))
	_, err := Start(context.Background(), Config{
		Protocol: "rdp", Host: "host.lan", Port: 3389, Runner: runner, OpenBrowser: true,
		Opener: func(context.Context, string) error { return fmt.Errorf("no browser") },
	})
	if err == nil || !strings.Contains(err.Error(), "no browser") || len(runner.snapshot()) != 9 {
		t.Fatalf("error/commands = %v %#v", err, runner.snapshot())
	}
}
