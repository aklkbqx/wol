package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadSizeBoundary(t *testing.T) {
	for _, size := range []int{0, 7, 8, 9, 32} {
		t.Run(strings.Repeat("x", size), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(bytes.Repeat([]byte{'x'}, size)) }))
			defer server.Close()
			c := NewClient("test", WithHTTPClient(server.Client()))
			got, err := c.downloadAsset(context.Background(), server.URL, 8)
			if size > 8 {
				if err == nil || !strings.Contains(err.Error(), "exceeds") || got != nil {
					t.Fatalf("oversized download: bytes=%d err=%v", len(got), err)
				}
			} else if err != nil || len(got) != size {
				t.Fatalf("within-limit download: bytes=%d err=%v", len(got), err)
			}
		})
	}
}

func TestExtractRejectsOversizedMetadataBeforeReading(t *testing.T) {
	for _, tc := range []struct {
		name, filename string
		size           int64
	}{
		{"binary", "wol", (128 << 20) + 1},
		{"discarded entry", "README", (256 << 20) + 1},
	} {
		t.Run("tar "+tc.name, func(t *testing.T) {
			var archive bytes.Buffer
			gz := gzip.NewWriter(&archive)
			tw := tar.NewWriter(gz)
			if err := tw.WriteHeader(&tar.Header{Name: tc.filename, Mode: 0755, Size: tc.size}); err != nil {
				t.Fatal(err)
			}
			// Deliberately no payload. Oversized metadata must be rejected before
			// attempting a huge read or reporting its truncated payload instead.
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			got, err := ExtractBinary(archive.Bytes(), "wol.tar.gz")
			if err == nil || !strings.Contains(err.Error(), "exceeds") || got != nil {
				t.Fatalf("got bytes=%d err=%v; want size-limit refusal", len(got), err)
			}
		})
	}
	t.Run("zip binary", func(t *testing.T) {
		var archive bytes.Buffer
		zw := zip.NewWriter(&archive)
		_, err := zw.CreateRaw(&zip.FileHeader{Name: "wol.exe", Method: zip.Store, UncompressedSize64: (128 << 20) + 1})
		if err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		got, err := ExtractBinary(archive.Bytes(), "wol.zip")
		if err == nil || !strings.Contains(err.Error(), "exceeds") || got != nil {
			t.Fatalf("got bytes=%d err=%v; want size-limit refusal", len(got), err)
		}
	})
}

func TestExtractionBudgets(t *testing.T) {
	for _, format := range []string{"tar.gz", "zip"} {
		for _, tc := range []struct {
			name                            string
			binarySize, readmeSize, entries int
			limits                          extractionLimits
			wantError                       string
		}{
			{"exact binary", 8, 0, 1, extractionLimits{8, 4096, 2}, ""},
			{"oversized binary", 9, 0, 1, extractionLimits{8, 4096, 2}, "exceeds"},
			{"entry budget", 8, 1, 3, extractionLimits{8, 4096, 2}, "entry count"},
			{"expansion budget", 8, 1024, 2, extractionLimits{8, 1024, 2}, "total"},
			{"empty binary", 0, 0, 1, extractionLimits{8, 4096, 2}, "empty"},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				var archive bytes.Buffer
				if format == "zip" {
					zw := zip.NewWriter(&archive)
					for i := 0; i < tc.entries; i++ {
						name, size := "README", tc.readmeSize
						if i == tc.entries-1 {
							name, size = "wol", tc.binarySize
						}
						w, err := zw.Create(name)
						if err != nil {
							t.Fatal(err)
						}
						if _, err := w.Write(bytes.Repeat([]byte("x"), size)); err != nil {
							t.Fatal(err)
						}
					}
					if err := zw.Close(); err != nil {
						t.Fatal(err)
					}
				} else {
					gz := gzip.NewWriter(&archive)
					tw := tar.NewWriter(gz)
					for i := 0; i < tc.entries; i++ {
						name, size := "README", tc.readmeSize
						if i == tc.entries-1 {
							name, size = "wol", tc.binarySize
						}
						if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(size), Mode: 0755}); err != nil {
							t.Fatal(err)
						}
						if _, err := tw.Write(bytes.Repeat([]byte("x"), size)); err != nil {
							t.Fatal(err)
						}
					}
					if err := tw.Close(); err != nil {
						t.Fatal(err)
					}
					if err := gz.Close(); err != nil {
						t.Fatal(err)
					}
				}
				got, err := extractBinaryWithLimits(archive.Bytes(), "wol."+format, tc.limits)
				if tc.wantError == "" {
					if err != nil || len(got) != tc.binarySize {
						t.Fatalf("bytes=%d err=%v", len(got), err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.wantError) || got != nil {
					t.Fatalf("bytes=%d err=%v want %s", len(got), err, tc.wantError)
				}
			})
		}
	}
}

func TestBoundedExtractionReaders(t *testing.T) {
	if got, err := ExtractBinary(nil, "wol_linux_amd64"); err == nil || got != nil {
		t.Fatal("empty raw executable accepted")
	}
	if got, err := readBoundedBinary(bytes.NewReader([]byte("123456789")), 8); err == nil || got != nil {
		t.Fatal("oversized stream accepted")
	}
	if got, err := readBoundedBinary(bytes.NewReader([]byte("12345678")), 8); err != nil || len(got) != 8 {
		t.Fatalf("exact stream: %v", err)
	}
	for _, size := range []int{8, 9} {
		r := &expansionReader{reader: bytes.NewReader(bytes.Repeat([]byte("x"), size)), remaining: 8}
		_, err := io.ReadAll(r)
		if (size == 8 && err != nil) || (size == 9 && (err == nil || !strings.Contains(err.Error(), "total"))) {
			t.Fatalf("size=%d err=%v", size, err)
		}
	}
}
