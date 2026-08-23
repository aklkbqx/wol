package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestUsage(t *testing.T) {
	var buf bytes.Buffer
	// usage writes to os.Stderr; redirect or call directly
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	usage()

	w.Close()
	os.Stderr = origStderr
	buf.ReadFrom(r)

	out := buf.String()
	if !strings.Contains(out, "wol tui") || !strings.Contains(out, "wol wake") || !strings.Contains(out, "wol remote") || !strings.Contains(out, "wol status") || !strings.Contains(out, "wol import") || !strings.Contains(out, "wol version") || strings.Contains(out, "wol server") || strings.Contains(out, "wol deploy") {
		t.Fatalf("unexpected usage output: %s", out)
	}
}

func TestPrintVersion(t *testing.T) {
	var out bytes.Buffer
	printVersion(&out)

	if got, want := out.String(), appVersion+"\nCredit: "+appCredit+"\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestWakeFlagValidation(t *testing.T) {
	// Test invalid destination IP
	code := runWake([]string{"--destination", "invalid-ip", "00:11:22:33:44:55"})
	if code != 2 {
		t.Errorf("expected exit code 2 for invalid IP, got %d", code)
	}

	// Test invalid MAC address
	code = runWake([]string{"--destination", "192.168.1.255", "invalid-mac"})
	if code != 2 {
		t.Errorf("expected exit code 2 for invalid MAC, got %d", code)
	}

	// Test missing MAC argument
	code = runWake([]string{})
	if code != 2 {
		t.Errorf("expected exit code 2 for missing MAC arg, got %d", code)
	}
}

func TestTUIFlagParsing(t *testing.T) {
	// Test invalid flag exits with code 2
	code := runWakeDesk([]string{"--unknown-flag=value"})
	if code != 2 {
		t.Errorf("expected exit code 2 for unknown flag, got %d", code)
	}
}
