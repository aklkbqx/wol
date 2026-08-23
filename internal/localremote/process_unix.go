//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly || solaris || aix

package localremote

import (
	"errors"
	"syscall"
)

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
