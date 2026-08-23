package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	config "github.com/aklkbqx/wol/internal/networkconfig"
)

// GetLocalIP returns non-loopback local IPv4
func GetLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}
	return "127.0.0.1"
}

// CheckTCPPort tests TCP connection with timeout
func CheckTCPPort(address string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// ScanNetworkTargets probes known network infrastructure concurrently
func ScanNetworkTargets(rootDir string) ([]DiscoveredTarget, error) {
	return ScanNetworkTargetsWithEnv(rootDir, nil)
}

// ScanNetworkTargetsWithEnv probes targets using dotenv defaults supplied by
// the caller while preserving process-environment precedence.
func ScanNetworkTargetsWithEnv(rootDir string, defaults []string) ([]DiscoveredTarget, error) {
	definitions, err := config.LoadNetworkTargetsWithEnv(rootDir, defaults)
	if err != nil {
		return nil, err
	}

	var results []DiscoveredTarget
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, definition := range definitions {
		wg.Add(1)
		go func(def config.NetworkTarget) {
			defer wg.Done()
			addr := net.JoinHostPort(def.Host, fmt.Sprintf("%d", def.Port))
			online := CheckTCPPort(addr, 600*time.Millisecond)

			status := "offline"
			if online {
				status = "online"
			}

			target := DiscoveredTarget{
				Type:         targetType(def.Type),
				Name:         def.Name,
				Host:         def.Host,
				IP:           def.Host,
				Port:         def.Port,
				Status:       status,
				SSHReachable: online,
				Details:      def.Details,
			}

			mu.Lock()
			results = append(results, target)
			mu.Unlock()
		}(definition)
	}

	wg.Wait()
	sort.Slice(results, func(i, j int) bool {
		if results[i].Type == results[j].Type {
			return results[i].Name < results[j].Name
		}
		return results[i].Type < results[j].Type
	})
	return results, nil
}

func targetType(value string) TargetType {
	switch value {
	case "router":
		return TargetRouter
	case "zerotier":
		return TargetZeroTier
	case "ssh-host":
		return TargetSSHHost
	default:
		return TargetServer
	}
}

// ScanIosSimulators parses xcrun simctl list devices
func ScanIosSimulators() []DiscoveredTarget {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "xcrun", "simctl", "list", "devices", "available", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var data struct {
		Devices map[string][]struct {
			Name        string `json:"name"`
			UDID        string `json:"udid"`
			State       string `json:"state"`
			IsAvailable bool   `json:"isAvailable"`
		} `json:"devices"`
	}

	if err := json.Unmarshal(out, &data); err != nil {
		return nil
	}

	var results []DiscoveredTarget
	for runtime, devs := range data.Devices {
		runtimeClean := strings.TrimPrefix(runtime, "com.apple.CoreSimulator.SimRuntime.")
		for _, dev := range devs {
			if dev.State == "Booted" {
				results = append(results, DiscoveredTarget{
					Type:         TargetIosSim,
					Name:         dev.Name,
					Host:         "localhost",
					IP:           "127.0.0.1",
					Port:         0,
					Status:       "booted",
					SSHReachable: false,
					Details:      "iOS Simulator (" + runtimeClean + ")",
					UDID:         dev.UDID,
				})
			}
		}
	}
	return results
}

// ScanAndroidDevices parses adb devices
func ScanAndroidDevices() []DiscoveredTarget {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "adb", "devices", "-l")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var results []DiscoveredTarget
	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] != "offline" && parts[1] != "unauthorized" {
			id := parts[0]
			isWifi := strings.Contains(id, ":")
			results = append(results, DiscoveredTarget{
				Type:         TargetAndroid,
				Name:         "Android " + id,
				Host:         id,
				IP:           id,
				Status:       "connected",
				SSHReachable: false,
				Details:      fmt.Sprintf("ADB (Wi-Fi: %t)", isWifi),
			})
		}
	}
	return results
}

// ScanAll executes all discovery mechanisms concurrently
func ScanAll(rootDir string) ([]DiscoveredTarget, error) {
	return ScanAllWithEnv(rootDir, nil)
}

// ScanAllWithEnv executes discovery using the caller's environment defaults.
func ScanAllWithEnv(rootDir string, defaults []string) ([]DiscoveredTarget, error) {
	var all []DiscoveredTarget
	var networkErr error
	var mu sync.Mutex
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		netTargets, err := ScanNetworkTargetsWithEnv(rootDir, defaults)
		if err != nil {
			networkErr = err
			return
		}
		mu.Lock()
		all = append(all, netTargets...)
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		sims := ScanIosSimulators()
		mu.Lock()
		all = append(all, sims...)
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		and := ScanAndroidDevices()
		mu.Lock()
		all = append(all, and...)
		mu.Unlock()
	}()

	wg.Wait()
	sort.Slice(all, func(i, j int) bool {
		if all[i].Type == all[j].Type {
			return all[i].Name < all[j].Name
		}
		return all[i].Type < all[j].Type
	})
	return all, networkErr
}
