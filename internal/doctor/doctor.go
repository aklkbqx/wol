package doctor

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	config "github.com/aklkbqx/wol/internal/networkconfig"
	"github.com/aklkbqx/wol/internal/ui"
)

// CheckItem represents a single system or network audit check
type CheckItem struct {
	Category string
	Name     string
	Status   string // "OK", "WARN", "FAIL"
	Details  string
}

// DoctorReport contains all audit results
type DoctorReport struct {
	Items []CheckItem
}

// RunDoctor performs a comprehensive environment audit
func RunDoctor(rootDir string) *DoctorReport {
	return RunDoctorWithEnv(rootDir, nil)
}

// RunDoctorWithEnv runs the audit with dotenv defaults supplied by the caller.
func RunDoctorWithEnv(rootDir string, defaults []string) *DoctorReport {
	report := &DoctorReport{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	add := func(cat, name, status, details string) {
		mu.Lock()
		report.Items = append(report.Items, CheckItem{
			Category: cat,
			Name:     name,
			Status:   status,
			Details:  details,
		})
		mu.Unlock()
	}

	// 1. Local wake toolchain checks
	tools := []struct{ name, cmd string }{
		{"SSH Client", "ssh"},
		{"Ping", "ping"},
		{"Local etherwake", "etherwake"},
		{"ZeroTier CLI", "zerotier-cli"},
	}

	for _, t := range tools {
		if path, err := exec.LookPath(t.cmd); err == nil {
			add("Toolchain", t.name, "OK", path)
		} else {
			add("Toolchain", t.name, "WARN", "Not found in PATH")
		}
	}

	// 2. Local ZeroTier Daemon Status
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "zerotier-cli", "status")
		out, err := cmd.Output()
		if err == nil && strings.Contains(string(out), "ONLINE") {
			parts := strings.Fields(string(out))
			ztID := ""
			if len(parts) >= 3 {
				ztID = parts[2]
			}
			add("ZeroTier", "Local ZeroTier Node", "OK", fmt.Sprintf("ONLINE (Node ID: %s)", ztID))
		} else {
			add("ZeroTier", "Local ZeroTier Node", "WARN", "Daemon not running or offline")
		}
	}()

	// 3. Network probes loaded from the same config used by wol scan.
	networkTargets, configErr := config.LoadNetworkTargetsWithEnv(rootDir, defaults)
	if configErr != nil {
		add("Configuration", "Network Targets", "FAIL", configErr.Error())
	} else if len(networkTargets) == 0 {
		add("Configuration", "Network Targets", "WARN", "No network targets configured")
	}

	for _, nt := range networkTargets {
		wg.Add(1)
		go func(name, host string, port int) {
			defer wg.Done()
			addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
			conn, err := net.DialTimeout("tcp", addr, 700*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				add("Network", name, "OK", fmt.Sprintf("Port %d Reachable", port))
			} else {
				add("Network", name, "WARN", fmt.Sprintf("Port %d Unreachable", port))
			}
		}(nt.Name, nt.Host, nt.Port)
	}

	// 4. Remote Router Etherwake Capability Check
	wg.Add(1)
	go func() {
		defer wg.Done()
		relayConfigured := false
		for _, target := range networkTargets {
			if !target.WOLRelay || target.SSHHost == "" {
				continue
			}
			relayConfigured = true
			out, err := findRemoteEtherwake(target.SSHHost)
			if err == nil && strings.Contains(string(out), "etherwake") {
				add("WOL Relay", "Router Etherwake Tool", "OK", fmt.Sprintf("%s via %s", strings.TrimSpace(string(out)), target.Name))
				return
			}
		}
		if relayConfigured {
			add("WOL Relay", "Router Etherwake Tool", "WARN", "SSH handshake or etherwake binary missing on configured relays")
		} else {
			add("WOL Relay", "Router Etherwake Tool", "WARN", "No WOL relay configured")
		}
	}()

	wg.Wait()
	sort.Slice(report.Items, func(i, j int) bool {
		if report.Items[i].Category == report.Items[j].Category {
			return report.Items[i].Name < report.Items[j].Name
		}
		return report.Items[i].Category < report.Items[j].Category
	})
	return report
}

func findRemoteEtherwake(host string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=2", host, "command -v etherwake")
	return cmd.Output()
}

// RenderReport formats the report into a styled terminal box
func (r *DoctorReport) RenderReport() string {
	var rows [][]string
	for _, it := range r.Items {
		badge := ui.Badge(it.Status, it.Status)
		rows = append(rows, []string{
			fmt.Sprintf("[%s] %s", it.Category, it.Name),
			fmt.Sprintf("%s  %s", badge, it.Details),
		})
	}
	return ui.RenderBox("WOL LOCAL NETWORK DOCTOR", rows)
}
