package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUpdateRequiresVerifiedArchive(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	binary := []byte("verified binary")
	if err := tw.WriteHeader(&tar.Header{Name: "wol", Size: int64(len(binary)), Mode: 0755}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	name := "wol_0.5.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	digest := fmt.Sprintf("%x", sha256.Sum256(archive.Bytes()))
	valid := digest + "  " + name + "\n"
	for _, tc := range []struct {
		name, manifestName, manifest, wantError string
		status                                  int
		tamper, checkOnly                       bool
		oversized                               bool
	}{
		{name: "valid", manifestName: "SHA256SUMS", manifest: valid},
		{name: "oversized extraction", manifestName: "SHA256SUMS", oversized: true, wantError: "exceeds"},
		{name: "legacy", manifestName: "wol_0.5.0_checksums.txt", manifest: valid},
		{name: "CRLF binary mode uppercase", manifestName: "SHA256SUMS", manifest: strings.ToUpper(digest) + " *" + name + "\r\n"},
		{name: "tampered", manifestName: "SHA256SUMS", manifest: valid, tamper: true, wantError: "checksum mismatch"},
		{name: "missing manifest", wantError: "checksum manifest"},
		{name: "wrong legacy version", manifestName: "wol_0.4.0_checksums.txt", manifest: valid, wantError: "checksum manifest"},
		{name: "missing entry", manifestName: "SHA256SUMS", manifest: digest + "  other.tar.gz\n", wantError: "missing checksum"},
		{name: "invalid digest", manifestName: "SHA256SUMS", manifest: strings.Repeat("z", 64) + "  " + name + "\n", wantError: "invalid checksum"},
		{name: "malformed", manifestName: "SHA256SUMS", manifest: "not a checksum\n", wantError: "invalid checksum"},
		{name: "duplicate", manifestName: "SHA256SUMS", manifest: valid + valid, wantError: "duplicate checksum"},
		{name: "unavailable", manifestName: "SHA256SUMS", status: http.StatusNotFound, wantError: "checksum manifest"},
		{name: "oversized manifest", manifestName: "SHA256SUMS", manifest: strings.Repeat(" ", (1<<20)+1), wantError: "exceeds"},
		{name: "check only", checkOnly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := append([]byte(nil), archive.Bytes()...)
			if tc.oversized {
				var oversized bytes.Buffer
				gz := gzip.NewWriter(&oversized)
				tw := tar.NewWriter(gz)
				if err := tw.WriteHeader(&tar.Header{Name: "wol", Size: (128 << 20) + 1, Mode: 0755}); err != nil {
					t.Fatal(err)
				}
				if err := gz.Close(); err != nil {
					t.Fatal(err)
				}
				data = oversized.Bytes()
				tc.manifest = fmt.Sprintf("%x  %s\n", sha256.Sum256(data), name)
			}
			var requests []string
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				switch r.URL.Path {
				case "/repos/aklkbqx/wol/releases/latest":
					assets := []map[string]any{{"name": name, "browser_download_url": server.URL + "/archive"}}
					if tc.manifestName != "" {
						assets = append(assets, map[string]any{"name": tc.manifestName, "browser_download_url": server.URL + "/checksums"})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.5.0", "assets": assets})
				case "/checksums":
					if tc.status != 0 {
						w.WriteHeader(tc.status)
					}
					_, _ = w.Write([]byte(tc.manifest))
				case "/archive":
					data := append([]byte(nil), data...)
					if tc.tamper {
						data = append(data, 'x')
					}
					_, _ = w.Write(data)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			target := filepath.Join(t.TempDir(), "wol")
			if err := os.WriteFile(target, []byte("old binary"), 0755); err != nil {
				t.Fatal(err)
			}
			client := NewClient("0.4.13", WithBaseAPIURL(server.URL), WithHTTPClient(server.Client()))
			_, err := client.ExecuteUpdate(context.Background(), UpdateOptions{TargetFile: target, CheckOnly: tc.checkOnly})
			if tc.wantError == "" && err != nil {
				t.Fatalf("update: %v", err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("got %v, want error containing %q", err, tc.wantError)
			}
			got, readErr := os.ReadFile(target)
			if readErr != nil {
				t.Fatal(readErr)
			}
			want := binary
			if tc.wantError != "" || tc.checkOnly {
				want = []byte("old binary")
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("installed binary = %q, want %q", got, want)
			}
			if tc.checkOnly && len(requests) != 1 {
				t.Fatalf("check-only downloaded assets: %v", requests)
			}
			if !tc.checkOnly && tc.wantError == "" && (len(requests) != 3 || requests[1] != "/checksums" || requests[2] != "/archive") {
				t.Fatalf("download order: %v", requests)
			}
		})
	}
}

func TestMacOSSourceFallbackPreservesPackaging(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS packaging policy")
	}
	t.Setenv("PATH", t.TempDir())
	_, err := BuildFromSource(context.Background(), "aklkbqx/wol", "v0.5.0")
	if err == nil || !strings.Contains(err.Error(), "macOS") || !strings.Contains(err.Error(), "make install") {
		t.Fatalf("got %v, want packaging-aware macOS instructions before looking for Go", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.0","assets":[]}`))
	}))
	defer server.Close()
	target := filepath.Join(t.TempDir(), "wol")
	if err := os.WriteFile(target, []byte("packaged original"), 0755); err != nil {
		t.Fatal(err)
	}
	client := NewClient("0.4.13", WithBaseAPIURL(server.URL), WithHTTPClient(server.Client()))
	_, err = client.ExecuteUpdate(context.Background(), UpdateOptions{TargetFile: target})
	if err == nil || !strings.Contains(err.Error(), "macOS") {
		t.Fatalf("update got %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "packaged original" {
		t.Fatal("failed fallback replaced packaged executable")
	}
}

func TestNonMacOSSourceFallbackRequiresGo(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS intentionally refuses automatic source builds")
	}
	t.Setenv("PATH", t.TempDir())
	_, err := BuildFromSource(context.Background(), "aklkbqx/wol", "v0.4.13")
	if err == nil || !strings.Contains(err.Error(), "go toolchain not found") {
		t.Fatalf("got %v, want supported source-build path to require Go", err)
	}
}
