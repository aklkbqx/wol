package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	wakeservice "github.com/aklkbqx/wol/internal/wake"
	"github.com/aklkbqx/wol/internal/wol"
	tea "github.com/charmbracelet/bubbletea"
)

// Uses real SQLite, routing, fleet and TUI state with synthetic network boundaries.
// It sends no packets, runs no SSH commands and does not replace first-user testing.
func TestThreeSiteInventoryBatchAcceptance(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	relay, err := repo.CreateWakeRelay(t.Context(), store.WakeRelay{Name: "unavailable-relay", Address: "198.51.100.1", Port: 22, Interface: "eth0", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	var sites []store.Site
	for i := 0; i < 3; i++ {
		s := store.Site{Name: fmt.Sprintf("site-%d", i), Subnet: "192.0.2.0/24", DefaultPort: 9, TimeoutMS: 100, Concurrency: 2}
		if i == 2 {
			s.WakeRelayID = relay.ID
		}
		s, err = repo.CreateSite(t.Context(), s)
		if err != nil {
			t.Fatal(err)
		}
		sites = append(sites, s)
	}
	for i := 0; i < 100; i++ {
		_, err = repo.CreateDevice(t.Context(), store.Device{Name: fmt.Sprintf("pc-%03d", i), MACAddress: fmt.Sprintf("02:00:00:00:00:%02x", i), IPAddress: fmt.Sprintf("192.0.2.%d", i+1), SiteID: sites[i%3].ID, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	m := NewWakeModel(repo, "test", "test")
	m.phase, m.loading = phaseReady, false
	m.motion = NewMotion(false)
	m.devices, err = repo.ListDevices(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	m.sites = sites
	m.detector = presence.NewDetector(presence.WithTCPPorts(nil), presence.WithPing(func(_ context.Context, host string, _ time.Duration) (time.Duration, error) {
		n, _ := strconv.Atoi(strings.TrimPrefix(host, "192.0.2."))
		if n%3 == 0 {
			return 0, errors.New("synthetic unreachable target")
		}
		return time.Millisecond, nil
	}))
	cmd := m.startPresenceScan(m.devices, m.requestID, loadingRefresh, nil)
	defer m.cancelLoading()
	m.filter = "pc-09"
	if got := len(m.filteredDevices()); got != 10 {
		t.Fatalf("search during scan matched %d", got)
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.selected != 1 {
		t.Fatal("navigation blocked during scan")
	}
	m.filter = ""
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
	online := 0
	for _, status := range m.presence {
		if status == "online" {
			online++
		}
	}
	if m.scanDone != 100 || online != 67 {
		t.Fatalf("mixed scan: completed=%d online=%d", m.scanDone, online)
	}

	var recovered atomic.Bool
	var directCalls, relayCalls atomic.Int32
	m.service = wakeservice.NewService(repo, wakeservice.Hooks{
		Direct: func(context.Context, wol.SendRequest) (wol.SendResult, error) {
			directCalls.Add(1)
			return wol.SendResult{Packets: 1}, nil
		},
		Relay: func(context.Context, store.WakeRelay, net.HardwareAddr) (wakeservice.RelayResult, error) {
			relayCalls.Add(1)
			if !recovered.Load() {
				return wakeservice.RelayResult{}, errors.New("synthetic relay unavailable")
			}
			return wakeservice.RelayResult{Packets: 1}, nil
		},
	})
	m.marked = make(map[string]bool)
	for _, d := range m.devices {
		m.marked[d.ID] = true
	}
	m.prepareBatch("wake")
	cmd = m.startBatch()
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
	sent, failed := 0, 0
	for _, r := range m.batchResults {
		switch r.Status {
		case "sent":
			sent++
		case "failed":
			failed++
		}
	}
	if sent != 67 || failed != 33 || directCalls.Load() != 67 || relayCalls.Load() != 33 {
		t.Fatalf("site isolation: sent=%d failed=%d direct=%d relay=%d", sent, failed, directCalls.Load(), relayCalls.Load())
	}
	m.retryBatch()
	if m.batchKind != "wake" || len(m.batchTargets) != 33 {
		t.Fatalf("retry preview: kind=%s targets=%d", m.batchKind, len(m.batchTargets))
	}
	for _, d := range m.batchTargets {
		if d.SiteID != sites[2].ID {
			t.Fatal("retry included a successful site")
		}
	}
	recovered.Store(true)
	cmd = m.startBatch()
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
	if len(m.batchResults) != 33 || directCalls.Load() != 67 || relayCalls.Load() != 66 {
		t.Fatal("retry repeated successful targets or lost failed targets")
	}
	for _, r := range m.batchResults {
		if r.Status != "sent" {
			t.Fatalf("recovered retry: %+v", r)
		}
	}

	backup := filepath.Join(t.TempDir(), "inventory.json")
	if err = inventoryFile(t.Context(), repo, exportForm, backup); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(filepath.Join(t.TempDir(), "restored.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if err = inventoryFile(t.Context(), restored, importForm, backup); err != nil {
		t.Fatal(err)
	}
	originalData, err := repo.Export(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	restoredData, err := restored.Export(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Import intentionally regenerates row IDs and timestamps; compare inventory
	// values and resolved relationships rather than those database bookkeeping fields.
	a, err := store.EncodeExport(acceptanceInventoryValues(originalData))
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.EncodeExport(acceptanceInventoryValues(restoredData))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("inventory restore changed exported devices, sites or relay relationships")
	}
}

func acceptanceInventoryValues(data store.ExportData) store.ExportData {
	relays := map[string]string{"": ""}
	sites := map[string]string{"": ""}
	for _, relay := range data.WakeRelays {
		relays[relay.ID] = relay.Name
	}
	for _, site := range data.Sites {
		sites[site.ID] = site.Name
	}
	for i := range data.WakeRelays {
		r := &data.WakeRelays[i]
		r.ID, r.CreatedAt, r.UpdatedAt = r.Name, "", ""
	}
	for i := range data.Sites {
		s := &data.Sites[i]
		s.ID, s.CreatedAt, s.UpdatedAt = s.Name, "", ""
		s.WakeRelayID = relays[s.WakeRelayID]
	}
	for i := range data.Devices {
		d := &data.Devices[i]
		d.ID, d.CreatedAt, d.UpdatedAt = d.MACAddress, "", ""
		d.SiteID, d.WakeRelayID = sites[d.SiteID], relays[d.WakeRelayID]
	}
	return data
}
