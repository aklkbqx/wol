package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/fleet"
	"github.com/aklkbqx/wol/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

type fleetMsg struct {
	id      uint64
	batch   bool
	result  fleet.Result
	done    bool
	results <-chan fleet.Result
}

func nextFleet(results <-chan fleet.Result, id uint64, batch bool) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-results
		return fleetMsg{id: id, batch: batch, result: r, done: !ok, results: results}
	}
}
func (m *WakeModel) applyFleetMsg(msg fleetMsg) tea.Cmd {
	if msg.batch {
		if msg.id != m.batchID {
			return nil
		}
		if msg.done {
			m.batchRunning = false
			m.batchKind = ""
			m.batchCancel()
			m.status = fmt.Sprintf("Batch complete: %d results. R retries failures; Esc clears selection.", len(m.batchResults))
			return nil
		}
		m.batchResults = append(m.batchResults, msg.result)
	} else if msg.id != m.requestID {
		return nil
	}
	if msg.done {
		m.checking = false
		m.checkedAt = time.Now()
		m.finishLoadContext()
		m.status = fmt.Sprintf("Checked %d/%d machines. Unreachable does not prove power is off.", m.scanDone, m.scanTotal)
		return nil
	}
	selectedID := ""
	if m.tab == 0 {
		if d, ok := m.selectedDevice(m.filteredDevices()); ok {
			selectedID = d.ID
		}
	}
	r := msg.result
	if !msg.batch || m.batchKind == "check" {
		m.ensurePresence()
		m.ensureCheckedDevice()
		if m.presenceMethod == nil {
			m.presenceMethod = make(map[string]string)
		}
		if m.presenceMessages == nil {
			m.presenceMessages = make(map[string]string)
		}
		if r.Status != "cancelled" {
			m.presence[r.DeviceID] = r.Status
			m.presenceMethod[r.DeviceID] = r.Method
			m.presenceMessages[r.DeviceID] = r.Message
			m.checkedDevice[r.DeviceID] = time.Now()
		}
	}
	if selectedID != "" {
		items := m.filteredDevices()
		m.selected = min(m.selected, max(0, len(items)-1))
		for i, d := range items {
			if d.ID == selectedID {
				m.selected = i
				break
			}
		}
	}
	if msg.batch {
		m.status = fmt.Sprintf("Batch %s %d/%d · %s: %s. Esc cancels.", m.batchKind, len(m.batchResults), len(m.batchTargets), r.Name, r.Status)
	} else {
		m.scanDone++
		m.status = fmt.Sprintf("Checking %d/%d · %s: %s. Esc cancels.", m.scanDone, m.scanTotal, r.Name, r.Status)
	}
	return nextFleet(msg.results, msg.id, msg.batch)
}
func (m *WakeModel) toggleMarked() {
	if m.tab != 0 {
		return
	}
	d, ok := m.selectedDevice(m.filteredDevices())
	if !ok {
		return
	}
	if m.marked == nil {
		m.marked = make(map[string]bool)
	}
	if m.marked[d.ID] {
		delete(m.marked, d.ID)
	} else {
		m.marked[d.ID] = true
	}
	m.status = fmt.Sprintf("%d selected across sites. W wake · S check · Esc clear.", len(m.marked))
}
func (m *WakeModel) prepareBatch(kind string) {
	if m.batchRunning {
		m.status = "A batch is already running. Esc cancels."
		return
	}
	m.batchPreviewIndex = 0
	m.batchTargets = nil
	for _, d := range m.devices {
		if m.marked[d.ID] {
			m.batchTargets = append(m.batchTargets, d)
		}
	}
	if len(m.batchTargets) == 0 {
		m.status = "Select machines with Space first."
		return
	}
	m.batchKind = kind
	m.status = fmt.Sprintf("Review %s for %d machines across sites. Enter confirms; Esc cancels.", kind, len(m.batchTargets))
}
func (m *WakeModel) startBatch() tea.Cmd {
	if len(m.batchTargets) == 0 {
		return nil
	}
	m.cancelLoading()
	m.batchRunning = true
	m.lastBatchKind = m.batchKind
	m.batchID++
	m.batchResults = nil
	ctx, cancel := context.WithCancel(context.Background())
	m.batchCancel = cancel
	profiles := make([]store.RemoteProfile, 0, len(m.profiles))
	for _, p := range m.profiles {
		profiles = append(profiles, p)
	}
	op := fleet.Check(m.presenceDetector(), profiles)
	if m.batchKind == "wake" {
		op = fleet.Wake(m.service)
	}
	results := fleet.Run(ctx, append([]store.Device(nil), m.batchTargets...), append([]store.Site(nil), m.sites...), 16, op)
	return nextFleet(results, m.batchID, true)
}
func (m *WakeModel) retryBatch() {
	if m.batchRunning {
		return
	}
	m.marked = make(map[string]bool)
	kind := m.lastBatchKind
	if kind == "" {
		kind = "check"
	}
	for _, r := range m.batchResults {
		if r.Status == "sent" || r.Status == "failed" {
			kind = "wake"
		}
		if r.Status != "sent" && r.Status != "online" {
			m.marked[r.DeviceID] = true
		}
	}
	m.prepareBatch(kind)
}
func (m *WakeModel) renderBatchPreview(width int) string {
	lines := []string{fmt.Sprintf("%s: %d selected machines", strings.ToUpper(m.batchKind), len(m.batchTargets))}
	sites := make(map[string]int)
	for _, d := range m.batchTargets {
		sites[m.siteName(d.SiteID)]++
	}
	// Inventory order is stable; keep the preview predictable.
	seen := make(map[string]bool)
	for _, d := range m.batchTargets {
		name := m.siteName(d.SiteID)
		if !seen[name] {
			lines = append(lines, fmt.Sprintf("%s: %d", name, sites[name]))
			seen[name] = true
		}
	}
	for i, d := range m.batchTargets {
		if i < m.batchPreviewIndex {
			continue
		}
		if i-m.batchPreviewIndex >= max(1, m.height-14) {
			lines = append(lines, "… remaining targets included in count")
			break
		}
		lines = append(lines, d.Name+" · "+m.siteName(d.SiteID))
	}
	lines = append(lines, "j/k scroll · Enter confirm · Esc cancel")
	for i := range lines {
		lines[i] = fitText(lines[i], width)
	}
	return strings.Join(lines, "\n")
}
func (m *WakeModel) siteName(id string) string {
	for _, s := range m.sites {
		if s.ID == id {
			return s.Name
		}
	}
	if id != "" {
		return id
	}
	return "Unassigned"
}
func (m *WakeModel) cycleSite() {
	ids := []string{"", "unassigned"}
	for _, s := range m.sites {
		ids = append(ids, s.ID)
	}
	i := 0
	for j, id := range ids {
		if id == m.siteFilter {
			i = j
			break
		}
	}
	m.siteFilter = ids[(i+1)%len(ids)]
	m.selected = 0
	m.status = "Site filter: " + m.siteName(m.siteFilter)
	if m.siteFilter == "" {
		m.status = "Site filter: all sites"
	}
}
func (m *WakeModel) cycleState() {
	states := []string{"", "online", "offline", "unknown"}
	i := 0
	for j, s := range states {
		if s == m.stateFilter {
			i = j
		}
	}
	m.stateFilter = states[(i+1)%len(states)]
	m.selected = 0
	m.status = "Status filter: " + m.stateFilter
	if m.stateFilter == "" {
		m.status = "Status filter: all"
	}
}

func (m *WakeModel) renderBatchResults(width int) string {
	rows := []string{fmt.Sprintf("Batch results %d/%d · j/k scroll · b back", len(m.batchResults), len(m.batchTargets))}
	start := max(0, m.batchPreviewIndex)
	end := min(len(m.batchResults), start+max(1, m.height-12))
	for _, r := range m.batchResults[start:end] {
		rows = append(rows, r.Name+" · "+r.Status+" · "+r.Method)
		if r.Message != "" {
			rows = append(rows, r.Message)
		}
	}
	if len(rows) > max(1, m.height-9) {
		rows = rows[:max(1, m.height-9)]
	}
	for i := range rows {
		rows[i] = fitText(rows[i], width)
	}
	return strings.Join(rows, "\n")
}
