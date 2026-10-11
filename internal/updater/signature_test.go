package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSignedManifestUpdatePolicy(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	binary := []byte("signed fixture binary")
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
	manifest := []byte(fmt.Sprintf("%x  %s\n", sha256.Sum256(archive.Bytes()), name))
	signature := ed25519.Sign(private, manifest)
	for _, tc := range []struct {
		name, wantError                                                                           string
		missing, corrupt, changedManifest, wrongKey, invalidKey, missingKey, noArchive, checkOnly bool
		signatureSize                                                                             int
		releaseTag                                                                                string
	}{
		{name: "valid"},
		{name: "replayed older manifest", releaseTag: "v0.6.0", wantError: "release version"},
		{name: "missing signature", missing: true, wantError: "signature"},
		{name: "corrupt signature", corrupt: true, wantError: "signature"},
		{name: "changed manifest", changedManifest: true, wantError: "signature"},
		{name: "wrong trusted key", wrongKey: true, wantError: "signature"},
		{name: "invalid key", invalidKey: true, wantError: "public key"},
		{name: "missing key file", missingKey: true, wantError: "public key"},
		{name: "oversized signature", signatureSize: 65, wantError: "signature"},
		{name: "truncated signature", signatureSize: 63, wantError: "signature"},
		{name: "signed mode refuses source fallback", noArchive: true, wantError: "signed"},
		{name: "check only skips verification", missing: true, checkOnly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := public
			if tc.wrongKey {
				key = other
			}
			der, err := x509.MarshalPKIXPublicKey(key)
			if err != nil {
				t.Fatal(err)
			}
			keyBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
			if tc.invalidKey {
				keyBytes = []byte("invalid key")
			}
			keyPath := filepath.Join(t.TempDir(), "trusted.pem")
			if !tc.missingKey {
				if err := os.WriteFile(keyPath, keyBytes, 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("WOL_UPDATE_PUBLIC_KEY", keyPath)
			archiveDownloaded := false
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/aklkbqx/wol/releases/latest":
					assets := []map[string]string{{"name": name, "browser_download_url": server.URL + "/archive"}, {"name": "SHA256SUMS", "browser_download_url": server.URL + "/manifest"}}
					if !tc.missing {
						assets = append(assets, map[string]string{"name": "SHA256SUMS.sig", "browser_download_url": server.URL + "/signature"})
					}
					if tc.noArchive {
						assets = nil
					}
					tag := tc.releaseTag
					if tag == "" {
						tag = "v0.5.0"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "assets": assets})
				case "/manifest":
					_, _ = w.Write(manifest)
					if tc.changedManifest {
						_, _ = w.Write([]byte("\n"))
					}
				case "/signature":
					sig := append([]byte(nil), signature...)
					if tc.corrupt {
						sig[0] ^= 1
					}
					if tc.signatureSize != 0 {
						sig = append(sig, 0)[:tc.signatureSize]
					}
					_, _ = w.Write(sig)
				case "/archive":
					archiveDownloaded = true
					_, _ = w.Write(archive.Bytes())
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			target := filepath.Join(t.TempDir(), "wol")
			if err := os.WriteFile(target, []byte("original"), 0755); err != nil {
				t.Fatal(err)
			}
			c := NewClient("0.4.14-dev", WithBaseAPIURL(server.URL), WithHTTPClient(server.Client()))
			_, err = c.ExecuteUpdate(t.Context(), UpdateOptions{TargetFile: target, CheckOnly: tc.checkOnly})
			if tc.wantError == "" && err != nil {
				t.Fatalf("update: %v", err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("got %v want %s", err, tc.wantError)
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" || tc.checkOnly {
				if string(got) != "original" || archiveDownloaded {
					t.Fatal("signature failure/check-only downloaded archive or replaced executable")
				}
			} else if !bytes.Equal(got, binary) || !archiveDownloaded {
				t.Fatal("verified update did not install binary")
			}
		})
	}
}
