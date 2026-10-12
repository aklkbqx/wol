package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aklkbqx/wol/internal/store"
)

func captureStdout(t *testing.T, run func() int) (int, string) {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	code := run()
	w.Close()
	os.Stdout = original
	return code, <-done
}

func seededInventory(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wol.db")
	t.Setenv("WOL_DB", filepath.Join(t.TempDir(), "unused.db"))
	repo, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	site, err := repo.CreateSite(t.Context(), store.Site{Name: "lab", BroadcastAddress: "10.0.0.255"})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []store.Device{
		{Name: "desk", MACAddress: "02:00:00:00:00:01", IPAddress: "10.0.0.10", SiteID: site.ID, Enabled: true},
		{Name: "nas", MACAddress: "02:00:00:00:00:02", IPAddress: "10.0.0.11", SiteID: site.ID, Enabled: true},
	} {
		if _, err := repo.CreateDevice(t.Context(), d); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestExportWritesInventoryThatImportRestores(t *testing.T) {
	path := seededInventory(t)
	code, out := captureStdout(t, func() int { return runExport([]string{"--db", path}) })
	var printed store.ExportData
	if code != 0 || json.Unmarshal([]byte(out), &printed) != nil || len(printed.Devices) != 2 || len(printed.Sites) != 1 {
		t.Fatalf("export to stdout = %d %q", code, out)
	}

	file := filepath.Join(t.TempDir(), "inventory.json")
	code, out = captureStdout(t, func() int { return runExport([]string{"--db", path, "--output", file}) })
	if code != 0 || !strings.Contains(out, "Exported 2 machines and 0 routes.") {
		t.Fatalf("export --output = %d %q", code, out)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("export file mode = %o, want 600 (exports hold connection settings)", mode)
	}

	restored := filepath.Join(t.TempDir(), "restored.db")
	if code, _ := captureStdout(t, func() int { return runImport([]string{file, "--db", restored}) }); code != 0 {
		t.Fatalf("import of export = %d", code)
	}
	repo, err := store.Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	devices, err := repo.ListDevices(t.Context())
	if err != nil || len(devices) != 2 {
		t.Fatalf("restored devices = %v %v", devices, err)
	}
	names := map[string]string{}
	for _, d := range devices {
		names[d.Name] = d.MACAddress
	}
	if names["desk"] != "02:00:00:00:00:01" || names["nas"] != "02:00:00:00:00:02" {
		t.Fatalf("restored inventory = %v", names)
	}
}

func TestExportRejectsArgumentsAndRefusesToOverwriteNothing(t *testing.T) {
	path := seededInventory(t)
	if code := runExport([]string{"--db", path, "extra"}); code != 2 {
		t.Fatalf("export with a positional argument = %d, want 2", code)
	}
	missingDir := filepath.Join(t.TempDir(), "missing", "inventory.json")
	if code, _ := captureStdout(t, func() int { return runExport([]string{"--db", path, "--output", missingDir}) }); code != 1 {
		t.Fatalf("export into a missing directory = %d, want 1", code)
	}
}

func TestShutdownStatusShowsRecordedHistory(t *testing.T) {
	path := seededInventory(t)
	code, out := captureStdout(t, func() int { return runShutdown([]string{"status", "--db", path}) })
	if code != 0 || !strings.Contains(out, "No power operation history recorded.") {
		t.Fatalf("empty status = %d %q", code, out)
	}
	repo, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []store.PowerAttempt{
		{DeviceName: "desk", Action: "schedule", DelaySeconds: 900, Status: "sent"},
		{DeviceName: "nas", Action: "shutdown_now", Status: "failed", Message: "ssh refused"},
		{DeviceName: "desk", Action: "cancel", Status: "sent"},
	} {
		if _, err := repo.RecordPowerAttempt(t.Context(), attempt); err != nil {
			t.Fatal(err)
		}
	}
	repo.Close()

	code, out = captureStdout(t, func() int { return runShutdown([]string{"status", "--db", path}) })
	if code != 0 || !strings.Contains(out, "POWER HISTORY") || !strings.Contains(out, "900s delay") || !strings.Contains(out, "ssh refused") || !strings.Contains(out, "FAILED") {
		t.Fatalf("status with history = %d %q", code, out)
	}
	code, out = captureStdout(t, func() int { return runShutdown([]string{"status", "--db", path, "--limit", "1"}) })
	if rows := strings.Count(out, "action: "); code != 0 || rows != 1 {
		t.Fatalf("status --limit 1 = %d with %d rows: %q", code, rows, out)
	}
}

func TestShutdownCancelNeedsOneKnownMachine(t *testing.T) {
	path := seededInventory(t)
	for _, args := range [][]string{
		{"--db", path},
		{"--db", path, "missing"},
		{"--db", path, "desk", "nas"},
	} {
		if code := runShutdown(append([]string{"cancel"}, args...)); code != 2 {
			t.Fatalf("shutdown cancel %v = %d, want 2", args, code)
		}
	}
	repo, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	attempts, err := repo.ListPowerAttempts(t.Context(), 10)
	if err != nil || len(attempts) != 0 {
		t.Fatalf("rejected cancels recorded attempts: %v %v", attempts, err)
	}
}
