// Package netutil contains IPv4 subnet calculations shared by discovery and sites.
package netutil

import (
	"fmt"
	"net"
	"strings"
)

func Broadcast(cidr string) (string, error) {
	ip, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil || ip.To4() == nil {
		return "", fmt.Errorf("subnet must be an IPv4 CIDR, for example 192.168.8.0/24")
	}
	ones, bits := network.Mask.Size()
	if bits != 32 || ones > 30 {
		return "", fmt.Errorf("subnet /%d has no broadcast destination", ones)
	}
	ip = network.IP.To4()
	for i := range ip {
		ip[i] |= ^network.Mask[i]
	}
	return ip.String(), nil
}

// LocalBroadcast uses the most specific connected subnet. An explicit interface
// restricts the lookup. No match returns empty, never a guessed /24.
func LocalBroadcast(host, iface string) string {
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil || ip.To4() == nil {
		return ""
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	best, result := -1, ""
	for _, item := range interfaces {
		if iface != "" && item.Name != iface {
			continue
		}
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 || item.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, err := item.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			_, network, err := net.ParseCIDR(addr.String())
			if err != nil || !network.Contains(ip) {
				continue
			}
			ones, bits := network.Mask.Size()
			if bits != 32 || ones <= best {
				continue
			}
			broadcast, err := Broadcast(network.String())
			if err == nil {
				best, result = ones, broadcast
			}
		}
	}
	return result
}
