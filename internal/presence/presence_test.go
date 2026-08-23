package presence

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestTargetValidation(t *testing.T) {
	d := NewDetector()
	ctx := context.Background()

	cases := []struct {
		name       string
		target     Target
		wantStatus Status
		wantMethod Method
	}{
		{
			name:       "empty IP",
			target:     Target{DeviceID: "d1", IPAddress: ""},
			wantStatus: StatusUnknown,
			wantMethod: MethodNone,
		},
		{
			name:       "whitespace IP",
			target:     Target{DeviceID: "d2", IPAddress: "   "},
			wantStatus: StatusUnknown,
			wantMethod: MethodNone,
		},
		{
			name:       "invalid format IP",
			target:     Target{DeviceID: "d3", IPAddress: "999.999.999.999"},
			wantStatus: StatusUnknown,
			wantMethod: MethodNone,
		},
		{
			name:       "unspecified IPv4",
			target:     Target{DeviceID: "d4", IPAddress: "0.0.0.0"},
			wantStatus: StatusUnknown,
			wantMethod: MethodNone,
		},
		{
			name:       "unspecified IPv6",
			target:     Target{DeviceID: "d5", IPAddress: "::"},
			wantStatus: StatusUnknown,
			wantMethod: MethodNone,
		},
		{
			name:       "broadcast IPv4",
			target:     Target{DeviceID: "d6", IPAddress: "255.255.255.255"},
			wantStatus: StatusUnknown,
			wantMethod: MethodNone,
		},
		{
			name:       "multicast IPv4",
			target:     Target{DeviceID: "d7", IPAddress: "224.0.0.1"},
			wantStatus: StatusUnknown,
			wantMethod: MethodNone,
		},
		{
			name:       "multicast IPv6",
			target:     Target{DeviceID: "d8", IPAddress: "ff02::1"},
			wantStatus: StatusUnknown,
			wantMethod: MethodNone,
		},
		{
			name:       "loopback IPv4 rejected by default",
			target:     Target{DeviceID: "d9", IPAddress: "127.0.0.1"},
			wantStatus: StatusUnknown,
			wantMethod: MethodNone,
		},
		{
			name:       "loopback IPv6 rejected by default",
			target:     Target{DeviceID: "d10", IPAddress: "::1"},
			wantStatus: StatusUnknown,
			wantMethod: MethodNone,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := d.Probe(ctx, tc.target, 500*time.Millisecond)
			if res.Status != tc.wantStatus {
				t.Errorf("got status %q, want %q", res.Status, tc.wantStatus)
			}
			if res.Method != tc.wantMethod {
				t.Errorf("got method %q, want %q", res.Method, tc.wantMethod)
			}
			if res.Message == "" {
				t.Errorf("expected validation message, got empty string")
			}
		})
	}
}

func TestTCPVerifyOpen(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	d := NewDetector(WithAllowLoopback(true))
	res := d.Probe(context.Background(), Target{
		DeviceID:   "dev-open",
		IPAddress:  "127.0.0.1",
		VerifyPort: port,
	}, 1*time.Second)

	if res.Status != StatusOnline {
		t.Fatalf("expected StatusOnline, got %q (message: %s)", res.Status, res.Message)
	}
	if res.Method != MethodTCPVerify {
		t.Fatalf("expected MethodTCPVerify, got %q", res.Method)
	}
	if res.DeviceID != "dev-open" {
		t.Fatalf("expected deviceId dev-open, got %q", res.DeviceID)
	}
	if res.IPAddress != "127.0.0.1" {
		t.Fatalf("expected ipAddress 127.0.0.1, got %q", res.IPAddress)
	}
}

func TestTCPVerifyRefused(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close() // Close port so connection will be refused (RST)

	d := NewDetector(WithAllowLoopback(true))
	res := d.Probe(context.Background(), Target{
		DeviceID:   "dev-refused",
		IPAddress:  "127.0.0.1",
		VerifyPort: port,
	}, 1*time.Second)

	if res.Status != StatusOnline {
		t.Fatalf("expected StatusOnline for TCP refused, got %q (message: %s)", res.Status, res.Message)
	}
	if res.Method != MethodTCPRefused {
		t.Fatalf("expected MethodTCPRefused, got %q", res.Method)
	}
}

func TestTCPSweepOpen(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	d := NewDetector(
		WithAllowLoopback(true),
		WithTCPPorts([]int{port}),
	)
	res := d.Probe(context.Background(), Target{
		DeviceID:   "dev-sweep-open",
		IPAddress:  "127.0.0.1",
		VerifyPort: 0, // No verify port configured, uses sweep
	}, 1*time.Second)

	if res.Status != StatusOnline {
		t.Fatalf("expected StatusOnline via sweep, got %q", res.Status)
	}
	if res.Method != MethodTCPSweep {
		t.Fatalf("expected MethodTCPSweep, got %q", res.Method)
	}
}

func TestTCPSweepRefused(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	d := NewDetector(
		WithAllowLoopback(true),
		WithTCPPorts([]int{port}),
	)
	res := d.Probe(context.Background(), Target{
		DeviceID:   "dev-sweep-refused",
		IPAddress:  "127.0.0.1",
		VerifyPort: 0,
	}, 1*time.Second)

	if res.Status != StatusOnline {
		t.Fatalf("expected StatusOnline via sweep refusal, got %q", res.Status)
	}
	if res.Method != MethodTCPSweep {
		t.Fatalf("expected MethodTCPSweep, got %q", res.Method)
	}
}

func TestICMPFallback(t *testing.T) {
	d := NewDetector(
		WithAllowLoopback(true),
		WithDialTCP(func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
			// Simulate timeout on all TCP dials
			return nil, errors.New("i/o timeout")
		}),
		WithPing(func(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
			return 12 * time.Millisecond, nil
		}),
	)

	res := d.Probe(context.Background(), Target{
		DeviceID:   "dev-icmp",
		IPAddress:  "192.168.1.50",
		VerifyPort: 445,
	}, 1*time.Second)

	if res.Status != StatusOnline {
		t.Fatalf("expected StatusOnline via ICMP, got %q", res.Status)
	}
	if res.Method != MethodICMP {
		t.Fatalf("expected MethodICMP, got %q", res.Method)
	}
	if res.LatencyMS != 12 {
		t.Fatalf("expected latency 12ms, got %d", res.LatencyMS)
	}
}

func TestAllProbesFailedOffline(t *testing.T) {
	d := NewDetector(
		WithAllowLoopback(true),
		WithDialTCP(func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
			return nil, errors.New("i/o timeout")
		}),
		WithPing(func(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
			return 0, errors.New("host unreachable")
		}),
	)

	res := d.Probe(context.Background(), Target{
		DeviceID:   "dev-offline",
		IPAddress:  "192.168.1.99",
		VerifyPort: 22,
	}, 500*time.Millisecond)

	if res.Status != StatusOffline {
		t.Fatalf("expected StatusOffline, got %q", res.Status)
	}
	if res.Method != MethodNone {
		t.Fatalf("expected MethodNone, got %q", res.Method)
	}
}

func TestBatchConcurrencyAndSummary(t *testing.T) {
	var activeGoroutines int64
	var maxActiveGoroutines int64

	d := NewDetector(
		WithConcurrency(3),
		WithTCPPorts([]int{80}),
		WithAllowLoopback(true),
		WithDialTCP(func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
			cur := atomic.AddInt64(&activeGoroutines, 1)
			defer atomic.AddInt64(&activeGoroutines, -1)

			for {
				max := atomic.LoadInt64(&maxActiveGoroutines)
				if cur <= max || atomic.CompareAndSwapInt64(&maxActiveGoroutines, max, cur) {
					break
				}
			}

			time.Sleep(20 * time.Millisecond)

			if address == "192.168.1.1:80" || address == "192.168.1.2:80" {
				return nil, syscall.ECONNREFUSED
			}
			return nil, errors.New("i/o timeout")
		}),
		WithPing(func(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
			if host == "192.168.1.3" {
				return 5 * time.Millisecond, nil
			}
			return 0, errors.New("timeout")
		}),
	)

	targets := []Target{
		{DeviceID: "d1", IPAddress: "192.168.1.1", VerifyPort: 80},  // online (refused)
		{DeviceID: "d2", IPAddress: "192.168.1.2", VerifyPort: 80},  // online (refused)
		{DeviceID: "d3", IPAddress: "192.168.1.3", VerifyPort: 900}, // online (icmp)
		{DeviceID: "d4", IPAddress: "192.168.1.4", VerifyPort: 900}, // offline
		{DeviceID: "d5", IPAddress: "invalid-ip", VerifyPort: 80},   // unknown
		{DeviceID: "d6", IPAddress: "", VerifyPort: 0},              // unknown
	}

	batch := d.ProbeBatch(context.Background(), targets, 1*time.Second)

	if batch.Summary.Total != 6 {
		t.Fatalf("expected total 6, got %d", batch.Summary.Total)
	}
	if batch.Summary.Online != 3 {
		t.Fatalf("expected online 3, got %d", batch.Summary.Online)
	}
	if batch.Summary.Offline != 1 {
		t.Fatalf("expected offline 1, got %d", batch.Summary.Offline)
	}
	if batch.Summary.Unknown != 2 {
		t.Fatalf("expected unknown 2, got %d", batch.Summary.Unknown)
	}

	// Verify order
	for i, tgt := range targets {
		if batch.Results[i].DeviceID != tgt.DeviceID {
			t.Fatalf("result %d deviceId %q != target deviceId %q", i, batch.Results[i].DeviceID, tgt.DeviceID)
		}
	}

	if maxActive := atomic.LoadInt64(&maxActiveGoroutines); maxActive > 3 {
		t.Fatalf("max concurrent goroutines exceeded: got %d, max allowed 3", maxActive)
	}
}

func TestIsConnectionRefused(t *testing.T) {
	if isConnectionRefused(nil) {
		t.Errorf("nil error should not be refused")
	}
	if !isConnectionRefused(syscall.ECONNREFUSED) {
		t.Errorf("syscall.ECONNREFUSED should be refused")
	}
	if !isConnectionRefused(errors.New("dial tcp 192.168.1.1:80: connect: connection refused")) {
		t.Errorf("text connection refused should be refused")
	}
	if !isConnectionRefused(errors.New("connectex: No connection could be made because the target machine actively refused it.")) {
		t.Errorf("windows actively refused text should be refused")
	}
	if isConnectionRefused(errors.New("i/o timeout")) {
		t.Errorf("timeout should not be refused")
	}
}
