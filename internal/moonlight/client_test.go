package moonlight

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBuildStreamArgs(t *testing.T) {
	client := &Client{ExecutablePath: "/usr/local/bin/moonlight"}
	args := client.BuildStreamArgs("192.168.8.200", "Desktop", 120, "2560x1440", 50000)
	expected := []string{"stream", "192.168.8.200", "Desktop", "--fps", "120", "--resolution", "2560x1440", "--bitrate", "50000"}
	assertArgs(t, args, expected)
}

func TestBuildStreamArgsDefaults(t *testing.T) {
	client := &Client{ExecutablePath: "/usr/local/bin/moonlight"}
	args := client.BuildStreamArgs("192.168.8.200", "", 0, "", 0)
	assertArgs(t, args, []string{"stream", "192.168.8.200", "Desktop"})
}

func TestBuildStreamArgsFlatpakPrefix(t *testing.T) {
	client := &Client{ExecutablePath: "/usr/bin/flatpak", FlatpakApp: "com.moonlight_stream.Moonlight"}
	args := client.BuildStreamArgs("10.0.0.8", "Desktop", 60, "1920x1080", 0)
	assertArgs(t, args, []string{"run", "com.moonlight_stream.Moonlight", "stream", "10.0.0.8", "Desktop", "--fps", "60", "--resolution", "1920x1080"})
}

func TestBuildPairArgs(t *testing.T) {
	client := &Client{ExecutablePath: "/usr/local/bin/moonlight"}
	assertArgs(t, client.BuildPairArgs("192.168.8.200", ""), []string{"pair", "192.168.8.200"})
	assertArgs(t, client.BuildPairArgs("192.168.8.200", "1234"), []string{"pair", "192.168.8.200", "--pin", "1234"})
}

func TestDetectWithLookPath(t *testing.T) {
	origLookPath := LookPathFunc
	defer func() { LookPathFunc = origLookPath }()

	LookPathFunc = func(file string) (string, error) {
		if file == "moonlight" {
			return "/usr/bin/moonlight", nil
		}
		return "", errors.New("not found")
	}

	client, err := Detect()
	if err != nil {
		t.Fatalf("expected client found, got err: %v", err)
	}
	if client.ExecutablePath != "/usr/bin/moonlight" {
		t.Fatalf("unexpected executable path: %s", client.ExecutablePath)
	}
}

func TestDetectNotFound(t *testing.T) {
	origLookPath := LookPathFunc
	origFileExists := FileExistsFunc
	defer func() {
		LookPathFunc = origLookPath
		FileExistsFunc = origFileExists
	}()

	LookPathFunc = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	FileExistsFunc = func(path string) bool {
		return false
	}

	client, err := Detect()
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got client: %+v, err: %v", client, err)
	}
}

func TestDetectIgnoresBareFlatpak(t *testing.T) {
	origLookPath := LookPathFunc
	origFileExists := FileExistsFunc
	defer func() {
		LookPathFunc = origLookPath
		FileExistsFunc = origFileExists
	}()
	LookPathFunc = func(file string) (string, error) {
		if file == "flatpak" {
			return "/usr/bin/flatpak", nil
		}
		return "", errors.New("not found")
	}
	FileExistsFunc = func(path string) bool { return false }
	if _, err := Detect(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("flatpak without moonlight app should be missing: %v", err)
	}
}

func assertArgs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("args length mismatch: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestDetectWithFileExistsFallback(t *testing.T) {
	origLookPath := LookPathFunc
	origFileExists := FileExistsFunc
	defer func() {
		LookPathFunc = origLookPath
		FileExistsFunc = origFileExists
	}()

	LookPathFunc = func(file string) (string, error) {
		if runtime.GOOS == "linux" && file == "flatpak" {
			return "/usr/bin/flatpak", nil
		}
		return "", errors.New("not found")
	}
	FileExistsFunc = func(path string) bool {
		return true
	}

	client, err := Detect()
	if err != nil || client == nil {
		t.Fatalf("Detect() fallback failed: client=%+v err=%v", client, err)
	}
}

func TestClientStartLaunchPairAndSessionLifecycle(t *testing.T) {
	var nilClient *Client
	if _, err := nilClient.Command(context.Background(), "192.168.1.10", "Desktop", 60, "1080p", 20000); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nilClient.Command err = %v, want ErrNotFound", err)
	}
	if err := nilClient.Pair(context.Background(), "192.168.1.10", "1234"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nilClient.Pair err = %v, want ErrNotFound", err)
	}

	var nilSession *Session
	if nilSession.Alive() {
		t.Fatal("nil session should not be alive")
	}
	if err := nilSession.Stop(); err != nil {
		t.Fatalf("nilSession.Stop() = %v", err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("POSIX script process test skipped on Windows")
	}

	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "moonlight-mock")
	script := `#!/bin/sh
if [ "$1" = "pair" ]; then
  if [ "$2" = "fail-host" ]; then
    exit 1
  fi
  exit 0
fi
if [ "$2" = "sleep-host" ]; then
  sleep 5
  exit 0
fi
exit 0
`
	if err := os.WriteFile(fakeBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if !FileExistsFunc(fakeBin) {
		t.Fatal("FileExistsFunc returned false for fake binary")
	}

	client := &Client{ExecutablePath: fakeBin}
	ctx := context.Background()

	if err := client.Pair(ctx, "192.168.1.10", "1234"); err != nil {
		t.Fatalf("Pair success path failed: %v", err)
	}
	if err := client.Pair(ctx, "fail-host", ""); err == nil {
		t.Fatal("Pair fail-host should return error")
	}

	if err := client.Launch(ctx, "192.168.1.10", "Desktop", 60, "1920x1080", 20000); err != nil {
		t.Fatalf("Launch failed: %v", err)
	}

	session, err := client.Start(ctx, "sleep-host", "Desktop", 60, "1920x1080", 20000)
	if err != nil {
		t.Fatalf("Start sleep-host failed: %v", err)
	}
	if !session.Alive() {
		t.Fatal("expected session to be alive immediately after Start")
	}
	if err := session.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if session.Alive() {
		t.Fatal("expected session to not be alive after Stop")
	}
	if err := session.Stop(); err != nil {
		t.Fatalf("second Stop() failed: %v", err)
	}
}
