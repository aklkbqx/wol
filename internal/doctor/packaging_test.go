package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const plistWithoutLocalNetwork = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>com.example.fixture</string>
</dict>
</plist>
`

// buildFixture compiles a tiny cgo program the way a Mac `go install` would,
// optionally embedding plist into __TEXT,__info_plist like `make build`.
func buildFixture(t *testing.T, plist string) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O fixtures need a macOS host")
	}
	if testing.Short() {
		t.Skip("builds a Go binary")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not in PATH")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\n\nimport \"C\"\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "fixture")
	args := []string{"build", "-o", out}
	if plist != "" {
		args = append(args, "-ldflags", "-linkmode=external -extldflags=-Wl,-sectcreate,__TEXT,__info_plist,"+plist)
	}
	args = append(args, src)
	cmd := exec.Command(goBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1", "GOFLAGS=")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, b)
	}
	return out
}

func repoPlist(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "packaging", "macos", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPackagingStatusSkipsOtherPlatforms(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		if _, _, applies := packagingStatus(goos, "/does/not/matter"); applies {
			t.Fatalf("packaging check applies on %s", goos)
		}
	}
}

func TestPackagingStatusWarnsWhenExecutableMissing(t *testing.T) {
	status, details, applies := packagingStatus("darwin", filepath.Join(t.TempDir(), "missing"))
	if !applies || status != "WARN" || !strings.Contains(details, "cannot inspect") {
		t.Fatalf("got applies=%v status=%q details=%q", applies, status, details)
	}
}

func TestPackagingStatusWarnsWhenFileIsNotMachO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wol")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec wol-real \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	status, details, _ := packagingStatus("darwin", path)
	if status != "WARN" || !strings.Contains(details, "cannot inspect") {
		t.Fatalf("got status=%q details=%q", status, details)
	}
}

func TestPackagingStatusWarnsWithoutInfoPlist(t *testing.T) {
	status, details, _ := packagingStatus("darwin", buildFixture(t, ""))
	if status != "WARN" || !strings.Contains(details, "make install") {
		t.Fatalf("got status=%q details=%q", status, details)
	}
}

func TestPackagingStatusWarnsWhenPlistLacksLocalNetworkKey(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mach-O fixtures need a macOS host")
	}
	plist := filepath.Join(t.TempDir(), "Info.plist")
	if err := os.WriteFile(plist, []byte(plistWithoutLocalNetwork), 0o600); err != nil {
		t.Fatal(err)
	}
	status, _, _ := packagingStatus("darwin", buildFixture(t, plist))
	if status != "WARN" {
		t.Fatalf("status = %q, want WARN", status)
	}
}

func TestPackagingStatusOKWithEmbeddedPlist(t *testing.T) {
	status, details, _ := packagingStatus("darwin", buildFixture(t, repoPlist(t)))
	if status != "OK" {
		t.Fatalf("status = %q details = %q", status, details)
	}
}

func TestPackagingStatusFollowsSymlink(t *testing.T) {
	target := buildFixture(t, repoPlist(t))
	link := filepath.Join(t.TempDir(), "wol")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if status, details, _ := packagingStatus("darwin", link); status != "OK" {
		t.Fatalf("status = %q details = %q", status, details)
	}
}

func TestAddPackagingNeverFails(t *testing.T) {
	var rows []CheckItem
	addPackaging(func(cat, name, status, details string) {
		rows = append(rows, CheckItem{Category: cat, Name: name, Status: status, Details: details})
	})
	if runtime.GOOS != "darwin" {
		if len(rows) != 0 {
			t.Fatalf("unexpected rows on %s: %+v", runtime.GOOS, rows)
		}
		return
	}
	if len(rows) != 1 || rows[0].Category != "Packaging" || rows[0].Name != "macOS Local Network" {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Status == "FAIL" {
		t.Fatalf("packaging check must not FAIL: %+v", rows[0])
	}
}
