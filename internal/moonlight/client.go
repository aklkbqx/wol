package moonlight

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// ErrNotFound is returned when the Moonlight client is not installed.
var ErrNotFound = errors.New("moonlight client not found (please install Moonlight from https://moonlight-stream.org)")

// Client provides discovery and stream execution for the Moonlight native client.
type Client struct {
	ExecutablePath string
	FlatpakApp     string
}

// Session is a launched Moonlight process that wol owns until Stop.
type Session struct {
	cmd     *exec.Cmd
	mu      sync.Mutex
	wait    sync.Once
	waitErr error
	done    chan struct{}
}

// LookPathFunc is a hook for exec.LookPath to allow unit testing.
var LookPathFunc = exec.LookPath

// FileExistsFunc is a hook for checking file existence to allow unit testing.
var FileExistsFunc = func(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Detect locates the Moonlight client on the host machine across macOS, Windows, and Linux.
func Detect() (*Client, error) {
	for _, name := range []string{"moonlight", "moonlight-qt", "Moonlight"} {
		if path, err := LookPathFunc(name); err == nil && path != "" {
			return &Client{ExecutablePath: path}, nil
		}
	}

	switch runtime.GOOS {
	case "darwin":
		for _, c := range []string{
			"/Applications/Moonlight.app/Contents/MacOS/Moonlight",
			"/Applications/Moonlight Game Streaming.app/Contents/MacOS/Moonlight",
			os.ExpandEnv("$HOME/Applications/Moonlight.app/Contents/MacOS/Moonlight"),
			os.ExpandEnv("$HOME/Applications/Moonlight Game Streaming.app/Contents/MacOS/Moonlight"),
		} {
			if FileExistsFunc(c) {
				return &Client{ExecutablePath: c}, nil
			}
		}
	case "windows":
		for _, c := range []string{
			`C:\Program Files\Moonlight Game Streaming\Moonlight.exe`,
			`C:\Program Files (x86)\Moonlight Game Streaming\Moonlight.exe`,
			os.ExpandEnv(`${LOCALAPPDATA}\Programs\Moonlight Game Streaming\Moonlight.exe`),
			os.ExpandEnv(`${PROGRAMFILES}\Moonlight Game Streaming\Moonlight.exe`),
		} {
			if FileExistsFunc(c) {
				return &Client{ExecutablePath: c}, nil
			}
		}
	case "linux":
		if path, err := LookPathFunc("flatpak"); err == nil && path != "" && FileExistsFunc("/var/lib/flatpak/app/com.moonlight_stream.Moonlight") {
			return &Client{ExecutablePath: path, FlatpakApp: "com.moonlight_stream.Moonlight"}, nil
		}
	}

	return nil, ErrNotFound
}

func (c *Client) prefixArgs() []string {
	if c != nil && c.FlatpakApp != "" {
		return []string{"run", c.FlatpakApp}
	}
	return nil
}

// BuildStreamArgs constructs moonlight-qt CLI arguments.
func (c *Client) BuildStreamArgs(host, appName string, fps int, resolution string, bitrateKbps int) []string {
	if strings.TrimSpace(appName) == "" {
		appName = "Desktop"
	}
	args := append(c.prefixArgs(), "stream", strings.TrimSpace(host), strings.TrimSpace(appName))
	if fps > 0 {
		args = append(args, "--fps", strconv.Itoa(fps))
	}
	if res := strings.TrimSpace(resolution); res != "" {
		args = append(args, "--resolution", strings.ToLower(res))
	}
	if bitrateKbps > 0 {
		args = append(args, "--bitrate", strconv.Itoa(bitrateKbps))
	}
	return args
}

// BuildPairArgs constructs moonlight-qt pairing arguments.
func (c *Client) BuildPairArgs(host, pin string) []string {
	args := append(c.prefixArgs(), "pair", strings.TrimSpace(host))
	if pin = strings.TrimSpace(pin); pin != "" {
		args = append(args, "--pin", pin)
	}
	return args
}

func (c *Client) executable() (string, error) {
	if c == nil || (c.ExecutablePath == "" && c.FlatpakApp == "") {
		return "", ErrNotFound
	}
	return c.ExecutablePath, nil
}

// Command creates an exec.Cmd configured to launch the stream.
// The process is not bound to ctx so a finished wake action does not kill it.
func (c *Client) Command(_ context.Context, host, appName string, fps int, resolution string, bitrateKbps int) (*exec.Cmd, error) {
	path, err := c.executable()
	if err != nil {
		return nil, err
	}
	return exec.Command(path, c.BuildStreamArgs(host, appName, fps, resolution, bitrateKbps)...), nil
}

// Start launches Moonlight and returns a session wol can stop later.
func (c *Client) Start(ctx context.Context, host, appName string, fps int, resolution string, bitrateKbps int) (*Session, error) {
	cmd, err := c.Command(ctx, host, appName, fps, resolution, bitrateKbps)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start moonlight stream: %w", err)
	}
	session := &Session{cmd: cmd, done: make(chan struct{})}
	go session.reap()
	return session, nil
}

// Launch starts Moonlight stream without retaining the process. Prefer Start.
func (c *Client) Launch(ctx context.Context, host, appName string, fps int, resolution string, bitrateKbps int) error {
	_, err := c.Start(ctx, host, appName, fps, resolution, bitrateKbps)
	return err
}

// Pair runs moonlight-qt pairing against host. An empty pin lets Moonlight prompt.
func (c *Client) Pair(ctx context.Context, host, pin string) error {
	path, err := c.executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, c.BuildPairArgs(host, pin)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("moonlight pair %s: %w", host, err)
	}
	return nil
}

func (s *Session) reap() {
	if s == nil || s.cmd == nil {
		return
	}
	s.wait.Do(func() {
		s.waitErr = s.cmd.Wait()
		if s.done != nil {
			close(s.done)
		}
	})
}

// Stop terminates the Moonlight process if it is still running.
func (s *Session) Stop() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if !s.Alive() {
		return nil
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	s.reap()
	return nil
}

// Alive reports whether the process has not exited yet.
func (s *Session) Alive() bool {
	if s == nil || s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	if s.done == nil {
		state := s.cmd.ProcessState
		return state == nil || !state.Exited()
	}
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}
