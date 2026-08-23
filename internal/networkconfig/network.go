package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultNetworkTargetsFile = "network-targets.json"

// NetworkTarget describes an endpoint used by the local scanner and doctor.
type NetworkTarget struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Type     string `json:"type"`
	Details  string `json:"details,omitempty"`
	SSHHost  string `json:"sshHost,omitempty"`
	WOLRelay bool   `json:"wolRelay,omitempty"`
}

type networkTargetsFile struct {
	Targets []NetworkTarget `json:"targets"`
}

// LoadNetworkTargets loads WOL_NETWORK_PROBE_TARGETS when set, otherwise the
// project network-targets JSON file. The environment format is name=host:port.
func LoadNetworkTargets(rootDir string) ([]NetworkTarget, error) {
	return LoadNetworkTargetsWithEnv(rootDir, nil)
}

// LoadNetworkTargetsWithEnv resolves process environment first, then the
// supplied dotenv defaults. It lets CLI commands share a selected env file
// without mutating the parent process.
func LoadNetworkTargetsWithEnv(rootDir string, defaults []string) ([]NetworkTarget, error) {
	if value := strings.TrimSpace(EnvValue("WOL_NETWORK_PROBE_TARGETS", defaults, "")); value != "" {
		return parseNetworkTargetsEnv(value)
	}

	configPath := strings.TrimSpace(EnvValue("WOL_NETWORK_TARGETS_FILE", defaults, ""))
	if configPath == "" {
		configPath = defaultNetworkTargetsFile
	}
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(rootDir, configPath)
	}

	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read network targets: %w", err)
	}

	var document networkTargetsFile
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse network targets: %w", err)
	}
	return validateNetworkTargets(document.Targets)
}

func parseNetworkTargetsEnv(value string) ([]NetworkTarget, error) {
	targets := make([]NetworkTarget, 0)
	for _, raw := range strings.Split(value, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		name, address := raw, raw
		if separator := strings.IndexByte(raw, '='); separator > 0 {
			name = strings.TrimSpace(raw[:separator])
			address = strings.TrimSpace(raw[separator+1:])
		}
		host, portValue, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid network target %q: %w", raw, err)
		}
		port, err := strconv.Atoi(portValue)
		if err != nil {
			return nil, fmt.Errorf("invalid port in network target %q", raw)
		}
		target := NetworkTarget{
			Name:    name,
			Host:    host,
			Port:    port,
			Type:    inferNetworkTargetType(name),
			Details: "Configured by WOL_NETWORK_PROBE_TARGETS",
		}
		if port == 22 && (target.Type == "router" || target.Type == "zerotier") {
			target.SSHHost = "root@" + host
			target.WOLRelay = true
		}
		targets = append(targets, target)
	}
	return validateNetworkTargets(targets)
}

func validateNetworkTargets(targets []NetworkTarget) ([]NetworkTarget, error) {
	seen := make(map[string]struct{}, len(targets))
	for index := range targets {
		target := &targets[index]
		target.Name = strings.TrimSpace(target.Name)
		target.Host = strings.TrimSpace(target.Host)
		target.Type = strings.ToLower(strings.TrimSpace(target.Type))
		if target.Name == "" || target.Host == "" {
			return nil, fmt.Errorf("network target %d requires name and host", index+1)
		}
		if target.Port < 1 || target.Port > 65535 {
			return nil, fmt.Errorf("network target %q has invalid port %d", target.Name, target.Port)
		}
		if target.Type == "" {
			target.Type = inferNetworkTargetType(target.Name)
		}
		if target.WOLRelay && strings.TrimSpace(target.SSHHost) == "" {
			target.SSHHost = "root@" + target.Host
		}
		switch target.Type {
		case "router", "zerotier", "server", "ssh-host":
		default:
			return nil, fmt.Errorf("network target %q has unsupported type %q", target.Name, target.Type)
		}
		key := net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate network target address %s", key)
		}
		seen[key] = struct{}{}
	}
	return targets, nil
}

func inferNetworkTargetType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "zerotier") || strings.HasPrefix(lower, "zt-") || strings.HasPrefix(lower, "zt_") || strings.Contains(lower, " zt"):
		return "zerotier"
	case strings.Contains(lower, "router") || strings.Contains(lower, "gateway"):
		return "router"
	default:
		return "server"
	}
}
