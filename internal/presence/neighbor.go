package presence

import (
	"context"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/aklkbqx/wol/internal/wol"
)

var (
	macPattern    = regexp.MustCompile(`(?i)(?:[0-9a-f]{1,2}[:-]){5}[0-9a-f]{1,2}`)
	ipv4Pattern   = regexp.MustCompile(`\b(\d{1,3}(?:\.\d{1,3}){3})\b`)
	darwinIface   = regexp.MustCompile(`(?i)\son\s+(\S+)`)
	linuxDevIface = regexp.MustCompile(`(?i)\sdev\s+(\S+)`)
)

// Neighbor is a complete ARP/neighbor entry on a physical interface.
type Neighbor struct {
	IP    string
	MAC   string
	Iface string
}

func ListNeighbors(ctx context.Context) ([]Neighbor, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "arp", "-a")
	case "linux":
		if _, err := exec.LookPath("ip"); err == nil {
			cmd = exec.CommandContext(ctx, "ip", "neigh", "show")
		} else {
			cmd = exec.CommandContext(ctx, "arp", "-an")
		}
	default:
		cmd = exec.CommandContext(ctx, "arp", "-an")
	}
	out, err := cmd.CombinedOutput()
	if len(out) == 0 && err != nil {
		return nil, err
	}
	return parseNeighborTable(runtime.GOOS, string(out)), nil
}

func defaultNeighbor(ctx context.Context, host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "arp", "-a", host)
	case "linux":
		if _, err := exec.LookPath("ip"); err == nil {
			cmd = exec.CommandContext(ctx, "ip", "neigh", "show", "to", host)
		} else {
			cmd = exec.CommandContext(ctx, "arp", "-n", host)
		}
	default:
		cmd = exec.CommandContext(ctx, "arp", "-n", host)
	}

	out, _ := cmd.CombinedOutput()
	return parseNeighborOutput(runtime.GOOS, string(out))
}

func parseNeighborOutput(goos, output string) bool {
	return len(parseNeighborTable(goos, output)) > 0
}

func parseNeighborTable(goos, output string) []Neighbor {
	local := localIPv4Set()
	seenMAC := make(map[string]struct{})
	var out []Neighbor
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "address") || strings.HasPrefix(lower, "interface:") {
			continue
		}
		if strings.Contains(lower, "incomplete") {
			continue
		}
		if strings.Contains(lower, "failed") {
			continue
		}
		macRaw := macPattern.FindString(line)
		if macRaw == "" || isIgnoredMAC(macRaw) {
			continue
		}
		parsed, err := wol.ParseMAC(macRaw)
		if err != nil {
			continue
		}
		mac := parsed.String()
		if _, dup := seenMAC[mac+"/"+ipv4Pattern.FindString(line)]; dup {
			continue
		}
		ip := ipv4Pattern.FindString(line)
		if ip == "" {
			continue
		}
		if _, isLocal := local[ip]; isLocal {
			continue
		}
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil || parsedIP.IsUnspecified() || parsedIP.IsMulticast() || parsedIP.IsLoopback() {
			continue
		}
		iface := neighborIface(line)
		if goos != "windows" && (iface == "" || !isPhysicalIface(iface)) {
			continue
		}
		seenMAC[mac+"/"+ip] = struct{}{}
		out = append(out, Neighbor{IP: ip, MAC: mac, Iface: iface})
	}
	return out
}

func localIPv4Set() map[string]struct{} {
	out := make(map[string]struct{})
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil {
			out[ip4.String()] = struct{}{}
		}
	}
	return out
}

func neighborIface(line string) string {
	if match := darwinIface.FindStringSubmatch(line); len(match) == 2 {
		return match[1]
	}
	if match := linuxDevIface.FindStringSubmatch(line); len(match) == 2 {
		return match[1]
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	last := fields[len(fields)-1]
	if strings.Contains(last, ":") || strings.Contains(last, ".") {
		return ""
	}
	return last
}

func isPhysicalIface(name string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.TrimSuffix(name, ":")
	if name == "" || name == "lo" || name == "lo0" {
		return false
	}
	virtualPrefixes := []string{
		"bridge", "br", "docker", "veth", "vmenet", "vmnet", "vboxnet",
		"feth", "utun", "awdl", "llw", "anpi", "gif", "stf", "ap",
		"virbr", "cni", "flannel", "cali", "tun", "tap", "wg", "zt",
		"tailscale", "cbridge", "p2p", "pdp", "awdl",
	}
	for _, prefix := range virtualPrefixes {
		if name == prefix || strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

func isIgnoredMAC(mac string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", ":", ".", ":").Replace(mac))
	parts := strings.Split(normalized, ":")
	if len(parts) != 6 {
		return true
	}
	allZero := true
	allFF := true
	for _, part := range parts {
		switch part {
		case "0", "00":
			allFF = false
		case "ff":
			allZero = false
		default:
			return false
		}
	}
	return allZero || allFF
}
