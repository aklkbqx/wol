package localremote

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMemoryVaultRoundTrip(t *testing.T) {
	vault := &MemoryVault{}
	key := VaultKey("rdp", "192.168.50.200", 3389)
	if _, err := vault.Get(key); !errors.Is(err, ErrVaultMiss) {
		t.Fatalf("empty vault err = %v", err)
	}
	saved := Credentials{Username: "desktop-user", Domain: "WORK", Password: "session-only"}
	if err := vault.Put(key, saved); err != nil {
		t.Fatal(err)
	}
	got, err := vault.Get(key)
	if err != nil || got != saved {
		t.Fatalf("got %+v err %v", got, err)
	}
	if err := vault.Delete(key); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Get(key); !errors.Is(err, ErrVaultMiss) {
		t.Fatalf("deleted vault err = %v", err)
	}
}

func TestConnectRemembersAndReusesKeychainSecret(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	vault := &MemoryVault{}
	server := httptest.NewUnstartedServer(nil)
	server.Listener.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	host := listener.Addr().String()
	cfg := Config{Name: "windows", Protocol: "rdp", Host: "192.168.50.200", Port: 3389, Vault: vault}
	server.Config.Handler = newBroker(host, "once", "cookie", "csrf", cfg, []byte("0123456789abcdef"), upstreamURL, func() {})
	server.Start()
	defer server.Close()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/s/once", nil)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	cookie := response.Cookies()[0]
	_ = response.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/session", nil)
	req.AddCookie(cookie)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "Remember on this Mac") {
		t.Fatalf("login remember prompt = %d %s", response.StatusCode, body)
	}

	req, _ = http.NewRequest(http.MethodPost, server.URL+"/connect", strings.NewReader("csrf=csrf&username=desktop-user&domain=WORK&password=session-only&remember=1&action=connect"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("remember connect status = %d", response.StatusCode)
	}
	saved, err := vault.Get(VaultKey("rdp", "192.168.50.200", 3389))
	if err != nil || saved.Username != "desktop-user" || saved.Password != "session-only" || saved.Domain != "WORK" {
		t.Fatalf("saved credentials = %+v err %v", saved, err)
	}

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/session", nil)
	req.AddCookie(cookie)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/remote" {
		t.Fatalf("saved session should auto-connect: status=%d location=%q", response.StatusCode, response.Header.Get("Location"))
	}

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/remote", nil)
	req.AddCookie(cookie)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "/guacamole/?data=") {
		t.Fatalf("auto-connect remote page = %d %s", response.StatusCode, body)
	}

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/session?manual=1", nil)
	req.AddCookie(cookie)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !strings.Contains(string(body), "Saved on this Mac") || !strings.Contains(string(body), "Forget saved sign-in") {
		t.Fatalf("manual login page = %s", body)
	}

	req, _ = http.NewRequest(http.MethodPost, server.URL+"/connect", strings.NewReader("csrf=csrf&action=forget"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "Saved sign-in removed") {
		t.Fatalf("forget status/body = %d %s", response.StatusCode, body)
	}
	if _, err := vault.Get(VaultKey("rdp", "192.168.50.200", 3389)); !errors.Is(err, ErrVaultMiss) {
		t.Fatalf("forget left secret in vault: %v", err)
	}
}

func TestSavedCredentialsSkipLoginOnOpen(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	upstreamURL, _ := url.Parse(upstream.URL)
	vault := &MemoryVault{}
	if err := vault.Put(VaultKey("ssh", "192.168.8.20", 22), Credentials{Username: "akalakkruaboon", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	server.Listener.Close()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	host := listener.Addr().String()
	cfg := Config{Name: "private2", Protocol: "ssh", Host: "192.168.8.20", Port: 22, Vault: vault}
	server.Config.Handler = newBroker(host, "once", "cookie", "csrf", cfg, []byte("0123456789abcdef"), upstreamURL, func() {})
	server.Start()
	defer server.Close()

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/s/once", nil)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	cookie := response.Cookies()[0]
	_ = response.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, server.URL+"/session", nil)
	req.AddCookie(cookie)
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/remote" {
		t.Fatalf("open with saved ssh should skip login: status=%d location=%q", response.StatusCode, response.Header.Get("Location"))
	}
}
