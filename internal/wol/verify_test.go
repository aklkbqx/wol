package wol

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"
)

func waitForTCPWithin(t *testing.T, limit time.Duration, ctx context.Context, host string, port int, timeout time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- WaitForTCP(ctx, host, port, timeout) }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		t.Fatalf("WaitForTCP did not return within %s", limit)
		return nil
	}
}

func TestWaitForTCPRetriesUntilThePortAccepts(t *testing.T) {
	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	reserved.Close()
	opened := make(chan net.Listener, 1)
	go func() {
		time.Sleep(1200 * time.Millisecond)
		listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			opened <- nil
			return
		}
		opened <- listener
	}()
	t.Cleanup(func() {
		if listener := <-opened; listener != nil {
			listener.Close()
		}
	})
	if err := waitForTCPWithin(t, 8*time.Second, context.Background(), "127.0.0.1", port, 6*time.Second); err != nil {
		t.Fatalf("WaitForTCP gave up before the machine came online: %v", err)
	}
}

func TestWaitForTCPStopsAtItsTimeout(t *testing.T) {
	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	reserved.Close()
	if err := waitForTCPWithin(t, 5*time.Second, context.Background(), "127.0.0.1", port, 1500*time.Millisecond); err == nil {
		t.Fatal("WaitForTCP reported a closed port as reachable")
	}
}

func TestWaitForTCPReturnsCloseToItsTimeout(t *testing.T) {
	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	reserved.Close()
	const timeout = 1500 * time.Millisecond
	start := time.Now()
	if err := waitForTCPWithin(t, 5*time.Second, context.Background(), "127.0.0.1", port, timeout); err == nil {
		t.Fatal("WaitForTCP reported a closed port as reachable")
	}
	if elapsed := time.Since(start); elapsed > timeout+400*time.Millisecond {
		t.Fatalf("WaitForTCP returned after %s, more than 400ms past its %s timeout", elapsed.Round(time.Millisecond), timeout)
	}
}

func TestWaitForTCPHonorsCancellation(t *testing.T) {
	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	reserved.Close()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	if err := waitForTCPWithin(t, 3*time.Second, ctx, "127.0.0.1", port, time.Minute); err != context.Canceled {
		t.Fatalf("WaitForTCP after cancel = %v, want context.Canceled", err)
	}
}

func TestWaitForTCPRejectsInvalidTargets(t *testing.T) {
	for _, target := range []struct {
		host string
		port int
	}{{"", 22}, {"127.0.0.1", 0}, {"127.0.0.1", 65536}} {
		if err := WaitForTCP(context.Background(), target.host, target.port, time.Second); err == nil {
			t.Fatalf("WaitForTCP(%q, %d) accepted an invalid target", target.host, target.port)
		}
	}
}
