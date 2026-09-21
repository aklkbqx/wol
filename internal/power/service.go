package power

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Action defines the power action performed.
type Action string

const (
	ActionShutdownNow Action = "shutdown_now"
	ActionSchedule    Action = "schedule"
	ActionCancel      Action = "cancel"
)

// Target represents the SSH and OS properties needed to execute a power command.
type Target struct {
	DeviceID   string
	DeviceName string
	Host       string
	Port       int
	User       string
	KeyPath    string
	Platform   string
	UseSudo    bool
}

// Request specifies the power operation parameters.
type Request struct {
	Target Target
	Delay  time.Duration
	Cancel bool
	Force  bool
}

// Result describes the outcome of a power operation.
type Result struct {
	Target        Target
	Action        Action
	Command       string
	Output        string
	Delay         time.Duration
	ScheduledTime time.Time
}

// SSHRunner abstracts the actual execution of an SSH command for testing and flexibility.
type SSHRunner func(ctx context.Context, target Target, command string) (string, error)

// Service coordinates power operations.
type Service struct {
	runner SSHRunner
}

// NewService creates a new power management service.
func NewService(runner SSHRunner) *Service {
	if runner == nil {
		runner = DefaultSSHRunner
	}
	return &Service{runner: runner}
}

// Execute performs the requested shutdown, schedule, or cancel operation.
func (s *Service) Execute(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.Target.Host) == "" {
		return Result{}, errors.New("remote target host cannot be empty")
	}
	if !safeSSHToken(req.Target.Host) {
		return Result{}, errors.New("remote target host contains unsafe characters")
	}
	if req.Target.User != "" && !safeSSHToken(req.Target.User) {
		return Result{}, errors.New("remote target user contains unsafe characters")
	}

	platform := NormalizePlatform(req.Target.Platform)
	if platform == "" {
		return Result{}, errors.New("target platform must be windows, linux, or darwin")
	}
	if req.Force && platform != PlatformWindows {
		return Result{}, errors.New("force is only supported on Windows")
	}
	if req.Delay > 0 && !req.Cancel {
		switch platform {
		case PlatformWindows:
			if int(req.Delay.Seconds()) < 1 {
				return Result{}, errors.New("windows delay must be at least 1s")
			}
		default:
			if req.Delay < time.Minute || req.Delay%time.Minute != 0 {
				return Result{}, errors.New("linux/darwin delay must be a whole number of minutes (e.g. 15m, 1h)")
			}
		}
	}

	driver := NewDriver(string(platform), req.Target.UseSudo)
	var command string
	var action Action
	var scheduledTime time.Time

	if req.Cancel {
		action = ActionCancel
		command = driver.BuildCancelCommand()
	} else if req.Delay > 0 {
		action = ActionSchedule
		command = driver.BuildShutdownCommand(req.Delay, req.Force)
		scheduledTime = time.Now().Add(req.Delay)
	} else {
		action = ActionShutdownNow
		command = driver.BuildShutdownCommand(0, req.Force)
	}

	out, err := s.runner(ctx, req.Target, command)
	res := Result{
		Target:        req.Target,
		Action:        action,
		Command:       command,
		Output:        strings.TrimSpace(out),
		Delay:         req.Delay,
		ScheduledTime: scheduledTime,
	}
	if err != nil {
		return res, err
	}
	return res, nil
}

// DefaultSSHRunner executes commands using the system `ssh` binary.
func DefaultSSHRunner(ctx context.Context, target Target, command string) (string, error) {
	port := target.Port
	if port <= 0 {
		port = 22
	}

	destination := strings.TrimSpace(target.Host)
	if target.User != "" {
		destination = strings.TrimSpace(target.User) + "@" + destination
	}

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-p", strconv.Itoa(port),
	}

	if target.KeyPath != "" {
		keyPath := strings.TrimSpace(target.KeyPath)
		if strings.HasPrefix(keyPath, "-") || !safeSSHPath(keyPath) {
			return "", fmt.Errorf("unsafe ssh key path")
		}
		args = append(args, "-i", keyPath)
	}

	args = append(args, "--", destination, command)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	output, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(output))
	if err != nil {
		if outStr != "" {
			return outStr, fmt.Errorf("ssh execution failed: %s (%w)", outStr, err)
		}
		return "", fmt.Errorf("ssh execution failed: %w", err)
	}
	return outStr, nil
}

func safeSSHToken(val string) bool {
	if val == "" || strings.HasPrefix(val, "-") {
		return false
	}
	for _, r := range val {
		if r <= ' ' || strings.ContainsRune("'\";&|$`\\<>\n\r", r) {
			return false
		}
	}
	return true
}

func safeSSHPath(val string) bool {
	if val == "" || strings.HasPrefix(val, "-") {
		return false
	}
	for _, r := range val {
		if r < ' ' || strings.ContainsRune("'\";&|$`<>\n\r", r) {
			return false
		}
	}
	return true
}
