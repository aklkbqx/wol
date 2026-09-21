// Package updater provides self-update and version checking functionality for WOL.
package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultRepo is the default GitHub repository for WOL.
	DefaultRepo = "aklkbqx/wol"

	// DefaultTimeout is the default HTTP timeout for update checks and downloads.
	DefaultTimeout = 30 * time.Second
)

// Release represents release metadata retrieved from GitHub.
type Release struct {
	Tag         string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	TarballURL  string    `json:"tarball_url"`
	ZipballURL  string    `json:"zipball_url"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
	Assets      []Asset   `json:"assets"`
}

// Asset represents a downloadable release artifact from GitHub.
type Asset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckResult contains the result of checking for an update.
type CheckResult struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	Release         *Release
}

// Client handles querying, downloading, and applying updates.
type Client struct {
	CurrentVersion string
	Repo           string
	HTTPClient     *http.Client
	BaseAPIURL     string
	BaseWebURL     string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.HTTPClient = client
		}
	}
}

// WithRepo sets a custom repository in owner/repo format.
func WithRepo(repo string) Option {
	return func(c *Client) {
		if strings.TrimSpace(repo) != "" {
			c.Repo = strings.TrimSpace(repo)
		}
	}
}

// WithBaseAPIURL sets the base API URL (useful for testing).
func WithBaseAPIURL(url string) Option {
	return func(c *Client) {
		c.BaseAPIURL = strings.TrimRight(url, "/")
	}
}

// WithBaseWebURL sets the base web URL (useful for testing redirects).
func WithBaseWebURL(url string) Option {
	return func(c *Client) {
		c.BaseWebURL = strings.TrimRight(url, "/")
	}
}

// NewClient creates a new updater client.
func NewClient(currentVersion string, opts ...Option) *Client {
	c := &Client{
		CurrentVersion: currentVersion,
		Repo:           DefaultRepo,
		HTTPClient: &http.Client{},
		BaseAPIURL: "https://api.github.com",
		BaseWebURL: "https://github.com",
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// CleanVersion normalizes a version string by stripping leading 'v' and whitespace.
func CleanVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

// CompareVersions compares two semantic version strings.
// Returns -1 if v1 < v2, 0 if v1 == v2, and 1 if v1 > v2.
func CompareVersions(v1, v2 string) int {
	c1 := CleanVersion(v1)
	c2 := CleanVersion(v2)

	if c1 == c2 {
		return 0
	}
	if c1 == "" {
		return -1
	}
	if c2 == "" {
		return 1
	}

	// Separate pre-release tags (e.g. 0.4.7-beta.1)
	base1, pre1 := splitPreRelease(c1)
	base2, pre2 := splitPreRelease(c2)

	parts1 := strings.Split(base1, ".")
	parts2 := strings.Split(base2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	// If bases are identical, a version with a prerelease is considered older than one without
	if pre1 != "" && pre2 == "" {
		return -1
	}
	if pre1 == "" && pre2 != "" {
		return 1
	}
	if pre1 < pre2 {
		return -1
	}
	if pre1 > pre2 {
		return 1
	}

	return 0
}

func splitPreRelease(v string) (string, string) {
	if idx := strings.Index(v, "-"); idx != -1 {
		return v[:idx], v[idx+1:]
	}
	return v, ""
}

// CheckLatest checks GitHub for the latest release and determines if an update is available.
func (c *Client) CheckLatest(ctx context.Context) (*CheckResult, error) {
	release, err := c.FetchLatestRelease(ctx)
	if err != nil {
		return nil, err
	}

	cleanLatest := CleanVersion(release.Tag)
	cleanCurrent := CleanVersion(c.CurrentVersion)
	updateAvailable := CompareVersions(cleanCurrent, cleanLatest) < 0

	return &CheckResult{
		CurrentVersion:  c.CurrentVersion,
		LatestVersion:   release.Tag,
		UpdateAvailable: updateAvailable,
		Release:         release,
	}, nil
}

// FetchLatestRelease retrieves the latest release info from GitHub API or web redirect fallback.
func (c *Client) FetchLatestRelease(ctx context.Context) (*Release, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/releases/latest", c.BaseAPIURL, c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "wol-updater/"+c.CurrentVersion)

	resp, err := c.HTTPClient.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var release Release
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return nil, fmt.Errorf("decode release metadata: %w", err)
		}
		return &release, nil
	}

	if resp != nil {
		_ = resp.Body.Close()
	}

	// Fallback: check redirect location from https://github.com/{repo}/releases/latest
	webURL := fmt.Sprintf("%s/%s/releases/latest", c.BaseWebURL, c.Repo)
	tag, err := c.fetchTagFromRedirect(ctx, webURL)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}

	return &Release{
		Tag:     tag,
		Name:    tag,
		HTMLURL: fmt.Sprintf("%s/%s/releases/tag/%s", c.BaseWebURL, c.Repo, tag),
	}, nil
}

// FetchReleaseByTag retrieves a specific release by tag name.
func (c *Client) FetchReleaseByTag(ctx context.Context, tag string) (*Release, error) {
	normTag := tag
	if !strings.HasPrefix(normTag, "v") && !strings.HasPrefix(normTag, "V") {
		normTag = "v" + normTag
	}

	apiURL := fmt.Sprintf("%s/repos/%s/releases/tags/%s", c.BaseAPIURL, c.Repo, normTag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create release tag request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "wol-updater/"+c.CurrentVersion)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release %s: %w", normTag, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var release Release
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return nil, fmt.Errorf("decode release metadata: %w", err)
		}
		return &release, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return &Release{
			Tag:     normTag,
			Name:    normTag,
			HTMLURL: fmt.Sprintf("%s/%s/releases/tag/%s", c.BaseWebURL, c.Repo, normTag),
		}, nil
	}
	return nil, fmt.Errorf("github returned HTTP %d for release %s", resp.StatusCode, normTag)
}

// UpdateOptions controls the behavior of an update operation.
type UpdateOptions struct {
	CheckOnly     bool
	Force         bool
	TargetVersion string
	TargetFile    string
}

// UpdateResult provides information about the executed update operation.
type UpdateResult struct {
	PreviousVersion string
	NewVersion      string
	InstalledPath   string
	CheckOnly       bool
	UpToDate        bool
	Release         *Release
}

// ExecuteUpdate coordinates checking, downloading/building, and installing the update.
func (c *Client) ExecuteUpdate(ctx context.Context, opts UpdateOptions) (*UpdateResult, error) {
	var release *Release
	var isNewer bool

	if strings.TrimSpace(opts.TargetVersion) != "" {
		rel, err := c.FetchReleaseByTag(ctx, opts.TargetVersion)
		if err != nil {
			return nil, err
		}
		release = rel
		cleanTarget := CleanVersion(release.Tag)
		cleanCurrent := CleanVersion(c.CurrentVersion)
		cmp := CompareVersions(cleanCurrent, cleanTarget)
		isNewer = cmp < 0
		if cmp == 0 && !opts.Force {
			return &UpdateResult{
				PreviousVersion: c.CurrentVersion,
				NewVersion:      release.Tag,
				CheckOnly:       opts.CheckOnly,
				UpToDate:        true,
				Release:         release,
			}, nil
		}
	} else {
		check, err := c.CheckLatest(ctx)
		if err != nil {
			return nil, err
		}
		release = check.Release
		isNewer = check.UpdateAvailable
		if !isNewer && !opts.Force {
			return &UpdateResult{
				PreviousVersion: c.CurrentVersion,
				NewVersion:      check.LatestVersion,
				CheckOnly:       opts.CheckOnly,
				UpToDate:        true,
				Release:         release,
			}, nil
		}
	}

	result := &UpdateResult{
		PreviousVersion: c.CurrentVersion,
		NewVersion:      release.Tag,
		Release:         release,
	}

	if opts.CheckOnly {
		result.CheckOnly = true
		result.UpToDate = !isNewer
		return result, nil
	}

	// 1. Check if a precompiled release asset matches current platform
	var binaryData []byte
	asset := MatchAsset(release.Assets, runtime.GOOS, runtime.GOARCH)
	if asset != nil {
		data, err := c.DownloadAsset(ctx, asset.BrowserDownloadURL)
		if err != nil {
			return nil, fmt.Errorf("download release asset %s: %w", asset.Name, err)
		}
		extracted, err := ExtractBinary(data, asset.Name)
		if err != nil {
			return nil, fmt.Errorf("extract binary from asset %s: %w", asset.Name, err)
		}
		binaryData = extracted
	} else {
		// 2. Fallback: compile from source via local Go toolchain
		data, err := BuildFromSource(ctx, c.Repo, release.Tag)
		if err != nil {
			return nil, fmt.Errorf("no prebuilt release asset for %s/%s, and build from source failed: %w", runtime.GOOS, runtime.GOARCH, err)
		}
		binaryData = data
	}

	// 3. Atomically replace executable
	installedPath, err := ReplaceExecutable(opts.TargetFile, binaryData)
	if err != nil {
		return nil, fmt.Errorf("install updated binary: %w", err)
	}

	result.InstalledPath = installedPath
	return result, nil
}


func (c *Client) fetchTagFromRedirect(ctx context.Context, url string) (string, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "wol-updater/"+c.CurrentVersion)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", errors.New("no redirect location found")
	}

	// Extract tag from URL: .../releases/tag/<tag>
	parts := strings.Split(loc, "/releases/tag/")
	if len(parts) == 2 && parts[1] != "" {
		return strings.TrimSpace(parts[1]), nil
	}

	return "", fmt.Errorf("unexpected redirect location: %s", loc)
}

// MatchAsset finds the most suitable release asset for the given OS and architecture.
func MatchAsset(assets []Asset, targetOS, targetArch string) *Asset {
	osSynonyms := map[string][]string{
		"darwin":  {"darwin", "macos", "osx"},
		"linux":   {"linux"},
		"windows": {"windows", "win", "win32", "win64"},
	}

	archSynonyms := map[string][]string{
		"arm64": {"arm64", "aarch64"},
		"amd64": {"amd64", "x86_64", "x64"},
		"386":   {"386", "i386", "x86"},
		"arm":   {"arm", "armv7", "armv6"},
	}

	osList := osSynonyms[targetOS]
	if len(osList) == 0 {
		osList = []string{targetOS}
	}

	archList := archSynonyms[targetArch]
	if len(archList) == 0 {
		archList = []string{targetArch}
	}

	// Negative filters
	var negativeOS []string
	for k, list := range osSynonyms {
		if k != targetOS {
			negativeOS = append(negativeOS, list...)
		}
	}

	var negativeArch []string
	for k, list := range archSynonyms {
		if k != targetArch {
			negativeArch = append(negativeArch, list...)
		}
	}

	for i := range assets {
		asset := &assets[i]
		nameLower := strings.ToLower(asset.Name)

		// Exclude checksums, signatures, SBOMs
		if strings.HasSuffix(nameLower, ".sha256") ||
			strings.HasSuffix(nameLower, ".md5") ||
			strings.HasSuffix(nameLower, ".txt") ||
			strings.HasSuffix(nameLower, ".sig") ||
			strings.HasSuffix(nameLower, ".sbom") {
			continue
		}

		tokens := filenameTokens(nameLower)

		// Check OS match
		hasOS := false
		for _, osTerm := range osList {
			if tokenContains(tokens, osTerm) {
				hasOS = true
				break
			}
		}
		if !hasOS {
			continue
		}

		// Ensure no other OS term conflicts
		hasConflictingOS := false
		for _, neg := range negativeOS {
			if tokenContains(tokens, neg) {
				hasConflictingOS = true
				break
			}
		}
		if hasConflictingOS {
			continue
		}

		// Check Arch match
		hasArch := false
		for _, archTerm := range archList {
			if tokenContains(tokens, archTerm) {
				hasArch = true
				break
			}
		}
		if !hasArch {
			continue
		}

		// Ensure no other Arch term conflicts
		hasConflictingArch := false
		for _, neg := range negativeArch {
			if tokenContains(tokens, neg) {
				hasConflictingArch = true
				break
			}
		}
		if hasConflictingArch {
			continue
		}

		return asset
	}

	return nil
}

func filenameTokens(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
}

func tokenContains(tokens []string, target string) bool {
	for _, tok := range tokens {
		if tok == target {
			return true
		}
	}
	return false
}

// DownloadAsset downloads the asset from the given URL.
func (c *Client) DownloadAsset(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("User-Agent", "wol-updater/"+c.CurrentVersion)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download asset returned HTTP %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return nil, fmt.Errorf("read download content: %w", err)
	}

	return buf.Bytes(), nil
}

// ExtractBinary extracts the executable named 'wol' (or 'wol.exe') from an archive,
// or returns the raw bytes if the file is an executable binary directly.
func ExtractBinary(content []byte, assetName string) ([]byte, error) {
	nameLower := strings.ToLower(assetName)

	if strings.HasSuffix(nameLower, ".tar.gz") || strings.HasSuffix(nameLower, ".tgz") {
		return extractFromTarGz(content)
	}

	if strings.HasSuffix(nameLower, ".zip") {
		return extractFromZip(content)
	}

	// Raw binary
	return content, nil
}

func extractFromTarGz(content []byte) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar entry: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		baseName := filepath.Base(header.Name)
		if isWolBinary(baseName) {
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, tr); err != nil {
				return nil, fmt.Errorf("extract binary from tar: %w", err)
			}
			return buf.Bytes(), nil
		}
	}

	return nil, errors.New("wol binary not found inside tar.gz archive")
}

func extractFromZip(content []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("create zip reader: %w", err)
	}

	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			continue
		}

		baseName := filepath.Base(file.Name)
		if isWolBinary(baseName) {
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("open zip entry: %w", err)
			}
			defer rc.Close()

			var buf bytes.Buffer
			if _, err := io.Copy(&buf, rc); err != nil {
				return nil, fmt.Errorf("extract binary from zip: %w", err)
			}
			return buf.Bytes(), nil
		}
	}

	return nil, errors.New("wol binary not found inside zip archive")
}

func isWolBinary(name string) bool {
	lower := strings.ToLower(name)
	return lower == "wol" || lower == "wol.exe"
}

// BuildFromSource compiles WOL from source using the local Go toolchain at the specified tag.
func BuildFromSource(ctx context.Context, repo, tag string) ([]byte, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "wol-build-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary build directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	normTag := tag
	if !strings.HasPrefix(normTag, "v") && !strings.HasPrefix(normTag, "V") {
		normTag = "v" + normTag
	}

	targetPackage := fmt.Sprintf("github.com/%s/cmd/wol@%s", repo, normTag)
	cmd := exec.CommandContext(ctx, goBin, "install", "-trimpath", "-ldflags=-s -w", targetPackage)
	cmd.Env = append(os.Environ(), "GOBIN="+tmpDir)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to build %s: %w\n%s", targetPackage, err, string(out))
	}

	binName := "wol"
	if runtime.GOOS == "windows" {
		binName = "wol.exe"
	}

	builtPath := filepath.Join(tmpDir, binName)
	data, err := os.ReadFile(builtPath)
	if err != nil {
		return nil, fmt.Errorf("read compiled binary: %w", err)
	}

	return data, nil
}

// IsTemporaryExecutable checks if the path appears to be a transient `go run` executable.
func IsTemporaryExecutable(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(lower, "/go-build") ||
		strings.Contains(lower, "\\go-build") ||
		strings.Contains(lower, "/tmp/go-") ||
		strings.Contains(lower, "\\tmp\\go-")
}

// ReplaceExecutable replaces the binary at targetPath atomically with newBinary bytes.
// If targetPath is empty, it uses os.Executable() and evaluates symlinks.
func ReplaceExecutable(targetPath string, newBinary []byte) (string, error) {
	if len(newBinary) == 0 {
		return "", errors.New("empty binary data")
	}

	if targetPath == "" {
		execPath, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("locate executable: %w", err)
		}
		targetPath = execPath
	}

	realPath, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		realPath = targetPath
	}
	realPath = filepath.Clean(realPath)

	if IsTemporaryExecutable(realPath) {
		return realPath, fmt.Errorf("current executable is running from a temporary directory (%s), likely via 'go run'. Specify a permanent destination with --target", realPath)
	}

	targetDir := filepath.Dir(realPath)

	tmpFile, err := os.CreateTemp(targetDir, ".wol-update-*")
	if err != nil {
		return realPath, fmt.Errorf("cannot create temporary file in %s (check write permissions): %w", targetDir, err)
	}
	tmpName := tmpFile.Name()

	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmpFile.Write(newBinary); err != nil {
		_ = tmpFile.Close()
		return realPath, fmt.Errorf("write binary content: %w", err)
	}

	if err := tmpFile.Chmod(0755); err != nil {
		_ = tmpFile.Close()
		return realPath, fmt.Errorf("chmod binary: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return realPath, fmt.Errorf("close temp binary: %w", err)
	}

	if runtime.GOOS == "windows" {
		oldPath := realPath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(realPath, oldPath); err != nil {
			return realPath, fmt.Errorf("move old binary on windows: %w", err)
		}
		if err := os.Rename(tmpName, realPath); err != nil {
			_ = os.Rename(oldPath, realPath)
			return realPath, fmt.Errorf("install new binary on windows: %w", err)
		}
		_ = os.Remove(oldPath)
	} else {
		if err := os.Rename(tmpName, realPath); err != nil {
			return realPath, fmt.Errorf("install new binary: %w", err)
		}
	}

	success = true
	return realPath, nil
}
