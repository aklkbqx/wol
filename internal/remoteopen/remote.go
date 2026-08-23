// Package remoteopen validates and hands remote-session URLs to the operating
// system. It deliberately stores no credentials and starts no web service.
package remoteopen

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

var allowedSchemes = map[string]bool{
	"http": true, "https": true,
}

// Validate accepts browser and native remote-session URLs without embedded
// credentials. The OS decides which installed application owns the scheme.
func Validate(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("remote URL is not configured")
	}
	if strings.ContainsAny(target, "\r\n\x00") {
		return "", errors.New("remote URL contains control characters")
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("remote URL must include a scheme and host")
	}
	if !allowedSchemes[strings.ToLower(parsed.Scheme)] {
		return "", fmt.Errorf("remote URL scheme %q is not supported", parsed.Scheme)
	}
	if parsed.User != nil {
		return "", errors.New("remote URL must not contain a username or password")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("remote URL must not contain a query string or fragment")
	}
	return parsed.String(), nil
}

// Command returns the shell-free OS handoff command for a validated URL.
func Command(ctx context.Context, target string) (*exec.Cmd, error) {
	validated, err := Validate(target)
	if err != nil {
		return nil, err
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.CommandContext(ctx, "open", validated), nil
	case "windows":
		return exec.CommandContext(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", validated), nil
	default:
		return exec.CommandContext(ctx, "xdg-open", validated), nil
	}
}

// Open hands the URL to the operating system and returns as soon as the
// external application starts.
func Open(ctx context.Context, target string) error {
	cmd, err := Command(ctx, target)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open remote session: %w", err)
	}
	return cmd.Process.Release()
}
