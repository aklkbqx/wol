package updater

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	maxArchiveBytes  = 128 << 20
	maxChecksumBytes = 1 << 20
)

// downloadVerifiedAsset checks the archive bytes before extraction or installation.
// These release checksums detect corruption; they are not independent signatures.
func (c *Client) downloadVerifiedAsset(ctx context.Context, release *Release, asset *Asset) ([]byte, error) {
	var manifest *Asset
	legacy := "wol_" + CleanVersion(release.Tag) + "_checksums.txt"
	for i := range release.Assets {
		candidate := &release.Assets[i]
		if candidate.Name == "SHA256SUMS" {
			manifest = candidate
			break
		}
		if candidate.Name == legacy {
			manifest = candidate
		}
	}
	if manifest == nil {
		return nil, fmt.Errorf("release %s has no checksum manifest; download and verify it manually before installation", release.Tag)
	}
	checksums, err := c.downloadAsset(ctx, manifest.BrowserDownloadURL, maxChecksumBytes)
	if err != nil {
		return nil, fmt.Errorf("download checksum manifest: %w", err)
	}
	expected, err := checksumForAsset(checksums, asset.Name)
	if err != nil {
		return nil, err
	}
	data, err := c.DownloadAsset(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return nil, err
	}
	actual := sha256.Sum256(data)
	if !bytes.Equal(actual[:], expected) {
		return nil, fmt.Errorf("checksum mismatch for %s; executable unchanged", asset.Name)
	}
	return data, nil
}

// checksumForAsset accepts GNU sha256sum text and binary-mode entries.
func checksumForAsset(data []byte, name string) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	seen := make(map[string]bool)
	var expected []byte
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < 67 || line[64] != ' ' || (line[65] != ' ' && line[65] != '*') {
			return nil, fmt.Errorf("invalid checksum manifest entry")
		}
		digest, err := hex.DecodeString(line[:64])
		if err != nil || len(digest) != sha256.Size {
			return nil, fmt.Errorf("invalid checksum manifest digest")
		}
		filename := line[66:]
		if filename == "" {
			return nil, fmt.Errorf("invalid checksum manifest filename")
		}
		if seen[filename] {
			return nil, fmt.Errorf("duplicate checksum for %s", filename)
		}
		seen[filename] = true
		if filename == name {
			expected = digest
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("invalid checksum manifest: %w", err)
	}
	if expected == nil {
		return nil, fmt.Errorf("missing checksum for %s", name)
	}
	return expected, nil
}
