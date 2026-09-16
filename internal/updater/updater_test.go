package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want int
	}{
		{"0.4.7", "0.4.7", 0},
		{"v0.4.7", "0.4.7", 0},
		{"0.4.7", "v0.4.7", 0},
		{"v0.4.7", "v0.4.8", -1},
		{"v0.4.8", "v0.4.7", 1},
		{"0.4.7", "0.5.0", -1},
		{"0.4.9", "0.4.10", -1},
		{"0.4.10", "0.4.9", 1},
		{"1.0.0", "0.4.7", 1},
		{"0.4.7-beta.1", "0.4.7", -1},
		{"0.4.7", "0.4.7-beta.1", 1},
		{"0.4.7-beta.1", "0.4.7-beta.2", -1},
		{"0.4.7-beta.2", "0.4.7-beta.1", 1},
		{"", "0.4.7", -1},
		{"0.4.7", "", 1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			got := CompareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestMatchAsset(t *testing.T) {
	assets := []Asset{
		{Name: "wol_0.4.8_checksums.txt"},
		{Name: "wol_0.4.8_darwin_amd64.tar.gz"},
		{Name: "wol_0.4.8_darwin_arm64.tar.gz"},
		{Name: "wol_0.4.8_linux_amd64.tar.gz"},
		{Name: "wol_0.4.8_linux_arm64.tar.gz"},
		{Name: "wol_0.4.8_windows_amd64.zip"},
	}

	tests := []struct {
		name       string
		targetOS   string
		targetArch string
		wantName   string
	}{
		{
			name:       "darwin arm64",
			targetOS:   "darwin",
			targetArch: "arm64",
			wantName:   "wol_0.4.8_darwin_arm64.tar.gz",
		},
		{
			name:       "darwin amd64",
			targetOS:   "darwin",
			targetArch: "amd64",
			wantName:   "wol_0.4.8_darwin_amd64.tar.gz",
		},
		{
			name:       "linux amd64",
			targetOS:   "linux",
			targetArch: "amd64",
			wantName:   "wol_0.4.8_linux_amd64.tar.gz",
		},
		{
			name:       "windows amd64",
			targetOS:   "windows",
			targetArch: "amd64",
			wantName:   "wol_0.4.8_windows_amd64.zip",
		},
		{
			name:       "unsupported freebsd",
			targetOS:   "freebsd",
			targetArch: "amd64",
			wantName:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := MatchAsset(assets, tt.targetOS, tt.targetArch)
			if tt.wantName == "" {
				if matched != nil {
					t.Errorf("expected no match, got %v", matched.Name)
				}
			} else {
				if matched == nil || matched.Name != tt.wantName {
					t.Errorf("MatchAsset() = %v, want %s", matched, tt.wantName)
				}
			}
		})
	}
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	content := []byte("#!/bin/sh\necho wol-binary\n")
	hdr := &tar.Header{
		Name: "wol",
		Mode: 0755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gzw.Close()

	extracted, err := ExtractBinary(buf.Bytes(), "wol_darwin_arm64.tar.gz")
	if err != nil {
		t.Fatalf("ExtractBinary failed: %v", err)
	}
	if !bytes.Equal(extracted, content) {
		t.Errorf("extracted content mismatch: got %q, want %q", string(extracted), string(content))
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	content := []byte("MZ-windows-wol-binary")
	w, err := zw.Create("wol.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	zw.Close()

	extracted, err := ExtractBinary(buf.Bytes(), "wol_windows_amd64.zip")
	if err != nil {
		t.Fatalf("ExtractBinary from zip failed: %v", err)
	}
	if !bytes.Equal(extracted, content) {
		t.Errorf("extracted content mismatch: got %q, want %q", string(extracted), string(content))
	}
}

func TestCheckLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/aklkbqx/wol/releases/latest" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"tag_name": "v0.4.8",
				"name": "WOL v0.4.8",
				"html_url": "https://github.com/aklkbqx/wol/releases/tag/v0.4.8",
				"assets": []
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClient("0.4.7",
		WithBaseAPIURL(server.URL),
		WithBaseWebURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	result, err := client.CheckLatest(context.Background())
	if err != nil {
		t.Fatalf("CheckLatest failed: %v", err)
	}

	if !result.UpdateAvailable {
		t.Errorf("expected update to be available for 0.4.7 -> 0.4.8")
	}
	if result.LatestVersion != "v0.4.8" {
		t.Errorf("expected latest version v0.4.8, got %s", result.LatestVersion)
	}

	// Test when already up to date
	clientSame := NewClient("0.4.8",
		WithBaseAPIURL(server.URL),
		WithBaseWebURL(server.URL),
		WithHTTPClient(server.Client()),
	)
	resultSame, err := clientSame.CheckLatest(context.Background())
	if err != nil {
		t.Fatalf("CheckLatest failed: %v", err)
	}
	if resultSame.UpdateAvailable {
		t.Errorf("expected no update available for 0.4.8 -> 0.4.8")
	}
}

func TestReplaceExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	binName := "wol-dummy"
	if runtime.GOOS == "windows" {
		binName = "wol-dummy.exe"
	}
	targetPath := filepath.Join(tmpDir, binName)

	// Create initial dummy binary
	initialContent := []byte("version-1")
	if err := os.WriteFile(targetPath, initialContent, 0755); err != nil {
		t.Fatal(err)
	}

	newContent := []byte("version-2")
	replacedPath, err := ReplaceExecutable(targetPath, newContent)
	if err != nil {
		t.Fatalf("ReplaceExecutable failed: %v", err)
	}
	realTarget, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		realTarget = targetPath
	}
	if replacedPath != realTarget {
		t.Errorf("replacedPath = %s, want %s", replacedPath, realTarget)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newContent) {
		t.Errorf("content after replacement = %q, want %q", string(got), string(newContent))
	}
}

func TestExecuteUpdateWithAsset(t *testing.T) {
	// Create mock binary tar.gz asset
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	content := []byte("#!/bin/sh\necho updated-wol\n")
	hdr := &tar.Header{
		Name: "wol",
		Mode: 0755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gzw.Close()
	assetBytes := buf.Bytes()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/aklkbqx/wol/releases/latest":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"tag_name": "v0.5.0",
				"name": "WOL v0.5.0",
				"html_url": "https://github.com/aklkbqx/wol/releases/tag/v0.5.0",
				"assets": [
					{
						"name": "wol_0.5.0_` + runtime.GOOS + `_` + runtime.GOARCH + `.tar.gz",
						"size": 100,
						"browser_download_url": "` + server.URL + `/download/wol.tar.gz"
					}
				]
			}`))
		case "/download/wol.tar.gz":
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(assetBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "wol")
	if err := os.WriteFile(targetFile, []byte("old-binary"), 0755); err != nil {
		t.Fatal(err)
	}

	client := NewClient("0.4.7",
		WithBaseAPIURL(server.URL),
		WithBaseWebURL(server.URL),
		WithHTTPClient(server.Client()),
	)

	// Test CheckOnly
	checkResult, err := client.ExecuteUpdate(context.Background(), UpdateOptions{
		CheckOnly:  true,
		TargetFile: targetFile,
	})
	if err != nil {
		t.Fatalf("ExecuteUpdate (check only) failed: %v", err)
	}
	if checkResult.UpToDate {
		t.Errorf("expected UpToDate false")
	}
	if checkResult.NewVersion != "v0.5.0" {
		t.Errorf("expected new version v0.5.0, got %s", checkResult.NewVersion)
	}

	// Test Actual Update with asset
	updateResult, err := client.ExecuteUpdate(context.Background(), UpdateOptions{
		TargetFile: targetFile,
	})
	if err != nil {
		t.Fatalf("ExecuteUpdate with asset failed: %v", err)
	}
	if updateResult.UpToDate {
		t.Errorf("expected UpToDate false after updating")
	}

	newBinary, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(newBinary, content) {
		t.Errorf("target file content mismatch: got %q, want %q", string(newBinary), string(content))
	}
}
