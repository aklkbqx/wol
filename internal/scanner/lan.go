package scanner

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/netutil"
	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/wol"
)

type LANHost struct {
	Neighbor presence.Neighbor
	Device   *store.Device
	Moved    bool
}

func ScanLAN(ctx context.Context) ([]presence.Neighbor, error) {
	return presence.ListNeighbors(ctx)
}

func ClassifyLAN(devices []store.Device, neighbors []presence.Neighbor) (known, moved []LANHost, unknown []presence.Neighbor) {
	byMAC := make(map[string]store.Device, len(devices))
	for _, device := range devices {
		mac, err := wol.ParseMAC(device.MACAddress)
		if err != nil {
			continue
		}
		byMAC[mac.String()] = device
	}
	for _, neighbor := range neighbors {
		device, ok := byMAC[neighbor.MAC]
		if !ok {
			unknown = append(unknown, neighbor)
			continue
		}
		host := LANHost{Neighbor: neighbor, Device: &device, Moved: device.IPAddress != neighbor.IP}
		if host.Moved {
			moved = append(moved, host)
		} else {
			known = append(known, host)
		}
	}
	return known, moved, unknown
}

func SyncDeviceIPs(ctx context.Context, repository *store.Store, devices []store.Device, neighbors []presence.Neighbor) (int, error) {
	if repository == nil {
		return 0, nil
	}
	_, moved, _ := ClassifyLAN(devices, neighbors)
	updated := 0
	counts := make(map[string]int)
	for _, n := range neighbors {
		counts[n.MAC]++
	}
	var siteByID map[string]store.Site
	for _, host := range moved {
		if host.Device != nil && host.Device.SiteID != "" {
			sites, err := repository.ListSites(ctx)
			if err != nil {
				return 0, err
			}
			siteByID = make(map[string]store.Site, len(sites))
			for _, site := range sites {
				siteByID[site.ID] = site
			}
			break
		}
	}
	for _, host := range moved {
		if host.Device == nil {
			continue
		}
		item := *host.Device
		if counts[host.Neighbor.MAC] != 1 || item.WakeRelayID != "" {
			continue
		}
		if item.SiteID != "" {
			site, ok := siteByID[item.SiteID]
			if !ok {
				continue
			}
			if site.WakeRelayID != "" {
				continue
			}
			if site.Subnet != "" {
				_, subnet, err := net.ParseCIDR(site.Subnet)
				if err != nil || !subnet.Contains(net.ParseIP(host.Neighbor.IP)) {
					continue
				}
			}
		}
		changed, err := repository.SyncDeviceAddress(ctx, item.ID, item.IPAddress, host.Neighbor.IP)
		if err != nil {
			return updated, err
		}
		if !changed {
			continue
		}
		updated++
	}
	return updated, nil
}

func BroadcastOf(ip string) string {
	return netutil.LocalBroadcast(ip, "")
}

func SuggestName(ip, mac string) string {
	lookupCtx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupAddr(lookupCtx, ip)
	if err == nil {
		for _, addr := range addrs {
			name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(addr)), ".")
			if name != "" && !strings.Contains(name, "in-addr.arpa") {
				if host, _, ok := strings.Cut(name, "."); ok && host != "" {
					return host
				}
				return name
			}
		}
	}
	parsed := net.ParseIP(ip).To4()
	if parsed != nil {
		return fmt.Sprintf("lan-%d-%d", parsed[2], parsed[3])
	}
	clean, err := wol.ParseMAC(mac)
	if err == nil {
		return "lan-" + strings.ReplaceAll(clean.String(), ":", "")[6:]
	}
	return "lan-host"
}

func GuessIdentity(ctx context.Context, ip string) (platform string, verifyPort int) {
	type hit struct {
		port     int
		platform string
	}
	probes := []hit{{3389, "windows"}, {22, "linux"}, {47989, "windows"}}
	dialer := net.Dialer{Timeout: 250 * time.Millisecond}
	for _, probe := range probes {
		if ctx.Err() != nil {
			return "unknown", 0
		}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(probe.port)))
		if err == nil {
			_ = conn.Close()
			return probe.platform, probe.port
		}
	}
	return "unknown", 0
}

func UniqueDeviceName(existing []store.Device, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "lan-host"
	}
	used := make(map[string]struct{}, len(existing))
	for _, device := range existing {
		used[strings.ToLower(device.Name)] = struct{}{}
	}
	if _, ok := used[strings.ToLower(name)]; !ok {
		return name
	}
	for i := 2; i < 100; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if _, ok := used[strings.ToLower(candidate)]; !ok {
			return candidate
		}
	}
	return name + "-new"
}
