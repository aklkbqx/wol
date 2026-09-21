package remoteopen

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aklkbqx/wol/internal/store"
)

func TestOpenDesktopWaitsForURLHandler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell URL handler fixture")
	}
	dir := t.TempDir()
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	launcher := filepath.Join(dir, name)
	t.Setenv("PATH", dir)
	profile := store.RemoteProfile{Protocol: "rdp", Host: "192.0.2.10", Port: 3389}
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\necho 'no registered client' >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := OpenDesktop(t.Context(), profile); err == nil || !strings.Contains(err.Error(), "no registered client") {
		t.Fatalf("handler failure must reach the caller: %v", err)
	}
	marker := filepath.Join(dir, "accepted")
	t.Setenv("WOL_TEST_LAUNCH_MARKER", marker)
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nprintf '%s' \"$1\" > \"$WOL_TEST_LAUNCH_MARKER\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	err := OpenDesktop(ctx, profile)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "rdp://192.0.2.10:3389" {
		t.Fatalf("launch was not accepted before return: %q, %v", got, err)
	}
	if err := OpenDesktop(ctx, profile); err != context.Canceled {
		t.Fatalf("cancelled launch = %v", err)
	}
}

func TestDesktopURLAndNativeMode(t *testing.T) {
	got, err := DesktopURL("rdp", "192.168.8.200", 3389, "")
	if err != nil || got != "rdp://192.168.8.200:3389" {
		t.Fatalf("rdp url = %q err=%v", got, err)
	}
	got, err = DesktopURL("ssh", "192.168.8.5", 22, "root")
	if err != nil || got != "ssh://root@192.168.8.5:22" {
		t.Fatalf("ssh url = %q err=%v", got, err)
	}
	if !NativeDesktop(store.RemoteProfile{Protocol: "rdp", Mode: "native"}) {
		t.Fatal("rdp native should use desktop client")
	}
	if NativeDesktop(store.RemoteProfile{Protocol: "rdp", Mode: "browser-local"}) {
		t.Fatal("browser-local rdp should stay on Guacamole")
	}
	if NativeDesktop(store.RemoteProfile{Protocol: "sunshine", Mode: "native-moonlight"}) {
		t.Fatal("sunshine should not use desktop open")
	}
}
