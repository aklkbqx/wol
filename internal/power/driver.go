package power

import (
	"fmt"
	"strings"
	"time"
)

// Platform identifies the remote target operating system.
type Platform string

const (
	PlatformWindows Platform = "windows"
	PlatformLinux   Platform = "linux"
	PlatformDarwin  Platform = "darwin"
)

// NormalizePlatform returns the normalized platform identifier.
func NormalizePlatform(val string) Platform {
	clean := strings.ToLower(strings.TrimSpace(val))
	switch clean {
	case "win", "windows":
		return PlatformWindows
	case "darwin", "mac", "macos", "osx":
		return PlatformDarwin
	default:
		return PlatformLinux
	}
}

// Driver represents an OS-specific command builder for power operations.
type Driver interface {
	BuildShutdownCommand(delay time.Duration, force bool) string
	BuildCancelCommand() string
}

// WindowsDriver formats shutdown commands for Windows hosts.
type WindowsDriver struct{}

func (d WindowsDriver) BuildShutdownCommand(delay time.Duration, force bool) string {
	seconds := int(delay.Seconds())
	if seconds < 0 {
		seconds = 0
	}
	flag := "/s"
	if force {
		flag += " /f"
	}
	return fmt.Sprintf("shutdown %s /t %d", flag, seconds)
}

func (d WindowsDriver) BuildCancelCommand() string {
	return "shutdown /a"
}

// LinuxDriver formats shutdown commands for Linux hosts.
type LinuxDriver struct {
	UseSudo bool
}

func (d LinuxDriver) BuildShutdownCommand(delay time.Duration, force bool) string {
	prefix := ""
	if d.UseSudo {
		prefix = "sudo "
	}
	if delay <= 0 {
		return prefix + "shutdown -h now"
	}
	minutes := int(delay.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("%sshutdown -h +%d", prefix, minutes)
}

func (d LinuxDriver) BuildCancelCommand() string {
	if d.UseSudo {
		return "sudo shutdown -c"
	}
	return "shutdown -c"
}

// DarwinDriver formats shutdown commands for macOS hosts.
type DarwinDriver struct {
	UseSudo bool
}

func (d DarwinDriver) BuildShutdownCommand(delay time.Duration, force bool) string {
	prefix := ""
	if d.UseSudo {
		prefix = "sudo "
	}
	if delay <= 0 {
		return prefix + "shutdown -h now"
	}
	minutes := int(delay.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("%sshutdown -h +%d", prefix, minutes)
}

func (d DarwinDriver) BuildCancelCommand() string {
	if d.UseSudo {
		return "sudo killall shutdown"
	}
	return "killall shutdown"
}

// NewDriver returns the appropriate Driver for the given platform.
func NewDriver(platform string, useSudo bool) Driver {
	switch NormalizePlatform(platform) {
	case PlatformWindows:
		return WindowsDriver{}
	case PlatformDarwin:
		return DarwinDriver{UseSudo: useSudo}
	default:
		return LinuxDriver{UseSudo: useSudo}
	}
}
