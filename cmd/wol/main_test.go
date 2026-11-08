package main

import (
	"bytes"
	"context"
	"flag"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aklkbqx/wol/internal/store"
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
	if !strings.Contains(out, "wol tui") || !strings.Contains(out, "wol wake") || !strings.Contains(out, "wol remote") || !strings.Contains(out, "wol status") || !strings.Contains(out, "wol import") || !strings.Contains(out, "wol update") || !strings.Contains(out, "wol version") || strings.Contains(out, "wol server") || strings.Contains(out, "wol deploy") {
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

func TestWakeAcceptsFlagsAfterMAC(t *testing.T) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := strconv.Itoa(listener.LocalAddr().(*net.UDPAddr).Port)
	if code := runWake([]string{"02:00:00:00:00:01", "--destination", "127.0.0.1", "--port", port, "--repeat", "1"}); code != 0 {
		t.Fatalf("wake with flags after the MAC exited %d, want 0", code)
	}
	if err := listener.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	packet := make([]byte, 256)
	if n, _, err := listener.ReadFromUDP(packet); err != nil || n != 102 {
		t.Fatalf("received %d bytes, %v; want one 102-byte magic packet on the requested port", n, err)
	}
}

func TestParseInterspersedKeepsPositionalsAfterTerminator(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	count := flags.Int("n", 0, "")
	positionals, err := parseInterspersed(flags, []string{"first", "-n", "2", "second", "--", "-literal"})
	if err != nil {
		t.Fatal(err)
	}
	if *count != 2 || strings.Join(positionals, ",") != "first,second,-literal" {
		t.Fatalf("n=%d positionals=%q, want n=2 and first,second,-literal", *count, positionals)
	}
}

func TestTUIFlagParsing(t *testing.T) {
	// Test invalid flag exits with code 2
	code := runWakeDesk([]string{"--unknown-flag=value"})
	if code != 2 {
		t.Errorf("expected exit code 2 for unknown flag, got %d", code)
	}
}

func TestWakeStoredDeviceAndDoctorAndScanFlags(t *testing.T) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.LocalAddr().(*net.UDPAddr).Port

	dbPath := filepath.Join(t.TempDir(), "wol.db")
	repo, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := repo.CreateDevice(context.Background(), store.Device{
		Name:             "stored-rig",
		MACAddress:       "02:00:00:00:00:99",
		BroadcastAddress: "127.0.0.1",
		Port:             port,
		Enabled:          false,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = repo.Close()

	// Waking disabled stored device without --force returns exit code 3
	if code := runWake([]string{"--db", dbPath, "--device", dev.Name, "--repeat", "1"}); code != 3 {
		t.Fatalf("runWake disabled device code = %d, want 3", code)
	}

	// Waking disabled stored device with --force returns exit code 0
	if code := runWake([]string{"--db", dbPath, "--device", dev.Name, "--repeat", "1", "--force"}); code != 0 {
		t.Fatalf("runWake --force stored device code = %d, want 0", code)
	}

	t.Setenv("TEST_WOL_ENV_HELPER", "  custom-val  ")
	if got := envString("TEST_WOL_ENV_HELPER", "fallback"); got != "custom-val" {
		t.Fatalf("envString = %q, want custom-val", got)
	}

	if code := runDoctor([]string{"--unknown-flag"}); code != 2 {
		t.Fatalf("runDoctor invalid flag code = %d, want 2", code)
	}
	if code := runScan([]string{"--unknown-flag"}); code != 2 {
		t.Fatalf("runScan invalid flag code = %d, want 2", code)
	}
	row := lanRow("192.168.1.10", "02:00:00:00:00:01", "box", "known")
	if len(row) != 2 || !strings.Contains(row[0], "box") {
		t.Fatalf("unexpected lanRow: %#v", row)
	}

	envPath := filepath.Join(t.TempDir(), ".wol.env")
	if err := os.WriteFile(envPath, []byte("WOL_DEFAULT_PORT=9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WOL_DB", dbPath)
	_ = runDoctor([]string{"--env-file", envPath})

	if code := runStatus([]string{"--db", dbPath}); code != 2 {
		t.Fatalf("runStatus missing arg code = %d, want 2", code)
	}
	if code := runStatus([]string{"--db", dbPath, "--json", dev.Name}); code != 1 {
		t.Fatalf("runStatus --json no-ip code = %d, want 1", code)
	}
}
