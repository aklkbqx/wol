package tui

import (
	"context"
	"fmt"
	"github.com/aklkbqx/wol/internal/fleet"
	"github.com/aklkbqx/wol/internal/presence"
	"github.com/aklkbqx/wol/internal/store"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSiteFormAssignmentAndBackup(t *testing.T) {
	repo, err := store.Open(filepath.Join(t.TempDir(), "wol.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	m := NewWakeModel(repo, "test", "test")
	m.phase = phaseReady
	m.tab = 3
	m.beginAdd()
	m.form = newWakeForm(siteForm, "", siteLabels(), []string{"Lab", "10.81.0.0/23", "", "", "", "9", "3000", "2"}, m.theme)
	result := m.saveForm()().(formSavedMsg)
	if result.keep {
		t.Fatal(result.message)
	}
	m.sites, err = repo.ListSites(t.Context())
	if err != nil || len(m.sites) != 1 {
		t.Fatal(err)
	}
	m.tab = 0
	m.beginAdd()
	m.form = newWakeForm(deviceForm, "", deviceFormLabels(), []string{"pc", "02:00:00:00:00:01", "10.81.0.10", "", "0", "", "3389", "broadcast", "", "Lab"}, m.theme)
	result = m.saveForm()().(formSavedMsg)
	if result.keep {
		t.Fatal(result.message)
	}
	devices, err := repo.ListDevices(t.Context())
	if err != nil || len(devices) != 1 {
		t.Fatal(err)
	}
	if devices[0].SiteID != m.sites[0].ID {
		t.Fatal("site assignment lost")
	}
	route, err := m.service.ResolveRoute(t.Context(), devices[0])
	if err != nil || route.Destination.String() != "10.81.1.255" {
		t.Fatalf("route=%v err=%v", route, err)
	}
	backup := filepath.Join(t.TempDir(), "inventory.json")
	if err = inventoryFile(t.Context(), repo, exportForm, backup); err != nil {
		t.Fatal(err)
	}
	if err = inventoryFile(t.Context(), repo, exportForm, backup); err == nil {
		t.Fatal("export overwrote existing backup")
	}
	dst, err := store.Open(filepath.Join(t.TempDir(), "copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if err = inventoryFile(t.Context(), dst, importForm, backup); err != nil {
		t.Fatal(err)
	}
	imported, _ := dst.ListDevices(t.Context())
	if len(imported) != 1 {
		t.Fatal(imported)
	}
	if err = repo.DeleteSite(t.Context(), m.sites[0].ID); err == nil {
		t.Fatal("deleted assigned site")
	}
}
func TestHundredMachineInteractionAndBatchCancellation(t *testing.T) {
	m := NewWakeModel(nil, "test", "test")
	m.phase = phaseReady
	m.motion = NewMotion(false)
	m.width = 100
	m.height = 30
	m.requestID = 1
	m.detector = presence.NewDetector(presence.WithTCPPorts(nil), presence.WithPing(func(context.Context, string, time.Duration) (time.Duration, error) { return time.Millisecond, nil }))
	for i := 0; i < 100; i++ {
		m.devices = append(m.devices, store.Device{ID: fmt.Sprint(i), Name: fmt.Sprintf("pc-%03d", i), IPAddress: "10.80.0.10", SiteID: fmt.Sprint(i % 3), Enabled: true})
	}
	m.sites = []store.Site{{ID: "0", Name: "Home", Concurrency: 2}, {ID: "1", Name: "Lab", Concurrency: 2}, {ID: "2", Name: "Office", Concurrency: 2}}
	cmd := m.startPresenceScan(m.devices, m.requestID, loadingRefresh, nil)
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.selected != 1 {
		t.Fatal("navigation blocked by scan")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.marked["1"] {
		t.Fatal("selection failed")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	if m.batchKind != "check" || len(m.batchTargets) != 1 {
		t.Fatal("batch was not previewed")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.batchKind != "" || m.batchRunning {
		t.Fatal("preview sent work")
	}
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
	if m.scanDone != 100 || m.checking {
		t.Fatalf("scan %d running=%v", m.scanDone, m.checking)
	}
	for _, w := range []int{18, 40, 80, 120} {
		m.width = w
		for tab := 0; tab < 4; tab++ {
			m.tab = tab
			for _, line := range strings.Split(m.View(), "\n") {
				if lipgloss.Width(stripANSI(line)) > w {
					t.Fatalf("width %d tab %d: %q", w, tab, line)
				}
			}
		}
	}
	m.tab = 0
	m.prepareBatch("check")
	cmd = m.startBatch()
	if cmd == nil {
		t.Fatal("missing batch")
	}
	m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	for cmd != nil {
		_, cmd = m.Update(cmd())
	}
	if m.batchRunning {
		t.Fatal("batch did not stop")
	}
}
func TestObsoleteStreamAndFilterKeepSelection(t *testing.T) {
	m := NewWakeModel(nil, "test", "test")
	m.phase = phaseReady
	m.requestID = 9
	m.devices = []store.Device{{ID: "a"}, {ID: "b"}}
	m.selected = 1
	m.applyFleetMsg(fleetMsg{id: 8, result: fleet.Result{DeviceID: "b", Status: "online"}})
	if m.presence["b"] != "" {
		t.Fatal("accepted obsolete result")
	}
	ch := make(chan fleet.Result)
	close(ch)
	m.applyFleetMsg(fleetMsg{id: 9, result: fleet.Result{DeviceID: "a", Status: "online"}, results: ch})
	if d, _ := m.selectedDevice(m.filteredDevices()); d.ID != "b" {
		t.Fatal("selection moved")
	}
}
