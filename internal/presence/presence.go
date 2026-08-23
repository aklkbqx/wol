package presence

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Status string

const (
	StatusOnline  Status = "online"
	StatusOffline Status = "offline"
	StatusUnknown Status = "unknown"
)

type Method string

const (
	MethodTCPVerify  Method = "tcp_verify"
	MethodTCPRefused Method = "tcp_refused"
	MethodTCPSweep   Method = "tcp_sweep"
	MethodICMP       Method = "icmp"
	MethodNone       Method = "none"
)

type Target struct {
	DeviceID   string `json:"deviceId"`
	IPAddress  string `json:"ipAddress"`
	VerifyPort int    `json:"verifyPort"`
}

type Result struct {
	DeviceID  string `json:"deviceId"`
	IPAddress string `json:"ipAddress"`
	Status    Status `json:"status"`
	Method    Method `json:"method"`
	LatencyMS int64  `json:"latencyMs"`
	CheckedAt string `json:"checkedAt"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Cached    bool   `json:"cached,omitempty"`
	Message   string `json:"message,omitempty"`
}

type Summary struct {
	Total   int `json:"total"`
	Online  int `json:"online"`
	Offline int `json:"offline"`
	Unknown int `json:"unknown"`
}

type BatchResult struct {
	CheckedAt  string   `json:"checkedAt"`
	DurationMS int64    `json:"durationMs"`
	Results    []Result `json:"results"`
	Summary    Summary  `json:"summary"`
}

type DialTCPFunc func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error)
type PingFunc func(ctx context.Context, host string, timeout time.Duration) (time.Duration, error)

type Options struct {
	TCPPorts      []int
	Concurrency   int
	AllowLoopback bool
	DialTCP       DialTCPFunc
	Ping          PingFunc
}

type Option func(*Options)

func WithTCPPorts(ports []int) Option {
	return func(o *Options) {
		o.TCPPorts = ports
	}
}

func WithConcurrency(concurrency int) Option {
	return func(o *Options) {
		o.Concurrency = concurrency
	}
}

func WithAllowLoopback(allow bool) Option {
	return func(o *Options) {
		o.AllowLoopback = allow
	}
}

func WithDialTCP(fn DialTCPFunc) Option {
	return func(o *Options) {
		o.DialTCP = fn
	}
}

func WithPing(fn PingFunc) Option {
	return func(o *Options) {
		o.Ping = fn
	}
}

type Detector struct {
	tcpPorts      []int
	concurrency   int
	allowLoopback bool
	dialTCP       DialTCPFunc
	pingFn        PingFunc
}

func NewDetector(opts ...Option) *Detector {
	options := Options{
		TCPPorts:    []int{22, 80, 443, 445, 3389, 5357, 8006},
		Concurrency: 16,
	}
	for _, opt := range opts {
		opt(&options)
	}

	ports := make([]int, len(options.TCPPorts))
	copy(ports, options.TCPPorts)

	return &Detector{
		tcpPorts:      ports,
		concurrency:   options.Concurrency,
		allowLoopback: options.AllowLoopback,
		dialTCP:       options.DialTCP,
		pingFn:        options.Ping,
	}
}

func (d *Detector) Probe(ctx context.Context, target Target, timeout time.Duration) Result {
	start := time.Now()
	checkedAt := start.UTC().Format(time.RFC3339Nano)

	if timeout <= 0 {
		timeout = 2000 * time.Millisecond
	}

	ip, err := validateTargetIP(target.IPAddress, d.allowLoopback)
	if err != nil {
		return Result{
			DeviceID:  target.DeviceID,
			IPAddress: target.IPAddress,
			Status:    StatusUnknown,
			Method:    MethodNone,
			LatencyMS: 0,
			CheckedAt: checkedAt,
			Message:   err.Error(),
		}
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	host := ip.String()

	// 1. Configured VerifyPort check
	if target.VerifyPort > 0 && target.VerifyPort <= 65535 {
		tcpTimeout := timeout / 3
		if tcpTimeout > 500*time.Millisecond {
			tcpTimeout = 500 * time.Millisecond
		}
		if tcpTimeout < 100*time.Millisecond {
			tcpTimeout = 100 * time.Millisecond
		}

		res := d.probeTCP(probeCtx, host, target.VerifyPort, tcpTimeout)
		if res.Online {
			method := MethodTCPVerify
			if res.Refused {
				method = MethodTCPRefused
			}
			return Result{
				DeviceID:  target.DeviceID,
				IPAddress: target.IPAddress,
				Status:    StatusOnline,
				Method:    method,
				LatencyMS: time.Since(start).Milliseconds(),
				CheckedAt: checkedAt,
			}
		}
	}

	// 2. Common TCP Sweep
	if len(d.tcpPorts) > 0 && probeCtx.Err() == nil {
		sweepTimeout := timeout / 2
		if sweepTimeout > 800*time.Millisecond {
			sweepTimeout = 800 * time.Millisecond
		}
		if sweepTimeout < 100*time.Millisecond {
			sweepTimeout = 100 * time.Millisecond
		}

		if d.sweepTCP(probeCtx, host, d.tcpPorts, sweepTimeout) {
			return Result{
				DeviceID:  target.DeviceID,
				IPAddress: target.IPAddress,
				Status:    StatusOnline,
				Method:    MethodTCPSweep,
				LatencyMS: time.Since(start).Milliseconds(),
				CheckedAt: checkedAt,
			}
		}
	}

	// 3. ICMP Ping fallback
	if probeCtx.Err() == nil {
		pingTimeout := 1000 * time.Millisecond
		if deadline, ok := probeCtx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining > 0 && remaining < pingTimeout {
				pingTimeout = remaining
			}
		}
		if pingTimeout < 100*time.Millisecond {
			pingTimeout = 100 * time.Millisecond
		}

		pingLatency, pingErr := d.ping(probeCtx, host, pingTimeout)
		if pingErr == nil {
			latencyMS := pingLatency.Milliseconds()
			if latencyMS == 0 {
				latencyMS = time.Since(start).Milliseconds()
			}
			return Result{
				DeviceID:  target.DeviceID,
				IPAddress: target.IPAddress,
				Status:    StatusOnline,
				Method:    MethodICMP,
				LatencyMS: latencyMS,
				CheckedAt: checkedAt,
			}
		}
	}

	return Result{
		DeviceID:  target.DeviceID,
		IPAddress: target.IPAddress,
		Status:    StatusOffline,
		Method:    MethodNone,
		LatencyMS: time.Since(start).Milliseconds(),
		CheckedAt: checkedAt,
		Message:   "target did not respond to TCP or ICMP probes",
	}
}

func (d *Detector) ProbeBatch(ctx context.Context, targets []Target, timeout time.Duration) BatchResult {
	start := time.Now()
	checkedAt := start.UTC().Format(time.RFC3339Nano)

	if timeout <= 0 {
		timeout = 2000 * time.Millisecond
	}

	results := make([]Result, len(targets))
	if len(targets) == 0 {
		return BatchResult{
			CheckedAt:  checkedAt,
			DurationMS: 0,
			Results:    results,
			Summary:    Summary{Total: 0, Online: 0, Offline: 0, Unknown: 0},
		}
	}

	concurrency := d.concurrency
	if concurrency <= 0 {
		concurrency = 16
	}

	if concurrency > len(targets) {
		concurrency = len(targets)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup

	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				results[idx] = d.Probe(ctx, targets[idx], timeout)
			}
		}()
	}
	for i := range targets {
		jobs <- i
	}
	close(jobs)

	wg.Wait()

	summary := d.Summarize(results)
	return BatchResult{
		CheckedAt:  checkedAt,
		DurationMS: time.Since(start).Milliseconds(),
		Results:    results,
		Summary:    summary,
	}
}

func (d *Detector) Summarize(results []Result) Summary {
	var summary Summary
	summary.Total = len(results)
	for _, res := range results {
		switch res.Status {
		case StatusOnline:
			summary.Online++
		case StatusOffline:
			summary.Offline++
		case StatusUnknown:
			summary.Unknown++
		}
	}
	return summary
}

type tcpProbeResult struct {
	Online  bool
	Refused bool
}

func (d *Detector) dial(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
	if d.dialTCP != nil {
		return d.dialTCP(ctx, network, address, timeout)
	}
	dialer := net.Dialer{Timeout: timeout}
	return dialer.DialContext(ctx, network, address)
}

func (d *Detector) probeTCP(ctx context.Context, host string, port int, timeout time.Duration) tcpProbeResult {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := d.dial(ctx, "tcp", address, timeout)
	if err == nil {
		_ = conn.Close()
		return tcpProbeResult{Online: true, Refused: false}
	}
	if isConnectionRefused(err) {
		return tcpProbeResult{Online: true, Refused: true}
	}
	return tcpProbeResult{Online: false, Refused: false}
}

func (d *Detector) sweepTCP(ctx context.Context, host string, ports []int, timeout time.Duration) bool {
	if len(ports) == 0 {
		return false
	}
	sweepCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultChan := make(chan bool, 1)
	var wg sync.WaitGroup

	for _, port := range ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			select {
			case <-sweepCtx.Done():
				return
			default:
			}
			res := d.probeTCP(sweepCtx, host, p, timeout)
			if res.Online {
				select {
				case resultChan <- true:
					cancel()
				default:
				}
			}
		}(port)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for found := range resultChan {
		if found {
			return true
		}
	}
	return false
}

func (d *Detector) ping(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
	if d.pingFn != nil {
		return d.pingFn(ctx, host, timeout)
	}
	return defaultPing(ctx, host, timeout)
}

func defaultPing(ctx context.Context, host string, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	timeoutSec := int(timeout.Seconds())
	if timeoutSec < 1 {
		timeoutSec = 1
	}
	timeoutMs := int(timeout.Milliseconds())
	if timeoutMs < 100 {
		timeoutMs = 100
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "ping", "-n", "1", "-w", strconv.Itoa(timeoutMs), host)
	case "darwin":
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(timeoutMs), host)
	default:
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(timeoutSec), host)
	}

	err := cmd.Run()
	if err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func validateTargetIP(ipStr string, allowLoopback bool) (net.IP, error) {
	trimmed := strings.TrimSpace(ipStr)
	if trimmed == "" {
		return nil, errors.New("device has no IP address configured")
	}
	ip := net.ParseIP(trimmed)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address %q", trimmed)
	}
	if ip.IsUnspecified() {
		return nil, errors.New("unspecified IP address is not allowed")
	}
	if ip.IsMulticast() {
		return nil, errors.New("multicast IP address is not allowed")
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4.Equal(net.IPv4bcast) || trimmed == "255.255.255.255" {
			return nil, errors.New("broadcast IP address is not allowed")
		}
	}
	if ip.IsLoopback() && !allowLoopback {
		return nil, errors.New("loopback IP address is not allowed")
	}
	return ip, nil
}

func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var sysErr syscall.Errno
	if errors.As(err, &sysErr) && sysErr == syscall.ECONNREFUSED {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "actively refused") ||
		strings.Contains(msg, "wsaeconnrefused")
}
