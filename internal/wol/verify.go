package wol

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"
)

func WaitForTCP(ctx context.Context, host string, port int, timeout time.Duration) error {
	if host == "" || port < 1 || port > 65535 {
		return fmt.Errorf("invalid TCP verification target")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("TCP port %s is not reachable", net.JoinHostPort(host, strconv.Itoa(port)))
		}
		attemptTimeout := 2 * time.Second
		if remaining < attemptTimeout {
			attemptTimeout = remaining
		}
		dialer := net.Dialer{Timeout: attemptTimeout}
		connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			connection.Close()
			return nil
		}
		retryWait := time.Second
		if remaining := time.Until(deadline); remaining < retryWait {
			retryWait = max(remaining, 0)
		}
		timer := time.NewTimer(retryWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
