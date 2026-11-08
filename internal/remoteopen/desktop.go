package remoteopen

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/store"
)

func DesktopProtocol(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "rdp", "ssh", "vnc":
		return true
	default:
		return false
	}
}

func NativeDesktop(profile store.RemoteProfile) bool {
	if !DesktopProtocol(profile.Protocol) {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(profile.Mode))
	return mode == "native" || mode == ""
}

func DesktopURL(protocol, host string, port int, user string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("desktop remote host is empty")
	}
	if strings.ContainsAny(host, "\r\n\x00") {
		return "", errors.New("desktop remote host contains control characters")
	}
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if !DesktopProtocol(protocol) {
		return "", fmt.Errorf("desktop protocol %q is not supported", protocol)
	}
	if port <= 0 {
		port = defaultDesktopPort(protocol)
	}
	hostPort := net.JoinHostPort(host, strconv.Itoa(port))
	switch protocol {
	case "ssh":
		ref := &url.URL{Scheme: "ssh", Host: hostPort}
		if user = strings.TrimSpace(user); user != "" {
			ref.User = url.User(user)
		}
		return ref.String(), nil
	case "vnc":
		return (&url.URL{Scheme: "vnc", Host: hostPort}).String(), nil
	default:
		return (&url.URL{Scheme: "rdp", Host: hostPort}).String(), nil
	}
}

func OpenDesktop(ctx context.Context, profile store.RemoteProfile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := DesktopURL(profile.Protocol, profile.Host, profile.Port, profile.UsernameHint)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" && strings.EqualFold(profile.Protocol, "rdp") {
		host := strings.TrimSpace(profile.Host)
		if profile.Port > 0 && profile.Port != 3389 {
			host = net.JoinHostPort(host, strconv.Itoa(profile.Port))
		}
		// The desktop session outlives the CLI's launch context. Release the
		// OS process handle directly instead of parking an unowned goroutine.
		cmd := exec.Command("mstsc", "/v:"+host)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("open Remote Desktop: %w", err)
		}
		if cmd.Process != nil {
			_ = cmd.Process.Release()
		}
		return nil
	}
	launchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(launchCtx, "open", target)
	case "windows":
		cmd = exec.CommandContext(launchCtx, "rundll32.exe", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.CommandContext(launchCtx, "xdg-open", target)
	}
	// Wait for the URL handler to accept the request before the caller cancels
	// its context, and surface failures such as a missing registered client.
	if output, err := cmd.CombinedOutput(); err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > 256 {
			detail = detail[:256]
		}
		return fmt.Errorf("open %s session: %w: %s", profile.Protocol, err, detail)
	}
	return nil
}

func defaultDesktopPort(protocol string) int {
	switch protocol {
	case "ssh":
		return 22
	case "vnc":
		return 5900
	default:
		return 3389
	}
}
