package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aklkbqx/wol/internal/fleet"
	"github.com/aklkbqx/wol/internal/store"
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
		m.checkedAt = m.now()
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
			m.checkedDevice[r.DeviceID] = m.now()
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

// runFleet starts a batch or presence scan. fleetRun replaces fleet.Run
// where nothing may reach the network, as in the website demo.
func (m *WakeModel) runFleet(ctx context.Context, devices []store.Device, sites []store.Site, workers int, op fleet.Operation) <-chan fleet.Result {
	if m.fleetRun != nil {
		return m.fleetRun(ctx, devices, sites, workers, op)
	}
	return fleet.Run(ctx, devices, sites, workers, op)
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
	if m.batchCancel != nil {
		m.batchCancel()
	}
	ctx, cancel := context.WithCancel(m.baseContext())
	m.batchCancel = cancel
	profiles := make([]store.RemoteProfile, 0, len(m.profiles))
	for _, p := range m.profiles {
		profiles = append(profiles, p)
	}
	op := fleet.Check(m.presenceDetector(), profiles)
	if m.batchKind == "wake" {
		op = fleet.Wake(m.service)
	}
	results := m.runFleet(ctx, append([]store.Device(nil), m.batchTargets...), append([]store.Site(nil), m.sites...), 16, op)
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
	budget := m.bodyLineBudget()
	framed := width >= 44 && budget >= 8
	innerW := width
	maxRows := max(4, budget)
	if framed {
		innerW = width - 4
		maxRows = max(4, budget-2)
	}

	lines := []string{m.theme.accent().Render(fitText(fmt.Sprintf("%s: %d selected machines", strings.ToUpper(m.batchKind), len(m.batchTargets)), innerW))}
	sites := make(map[string]int)
	for _, d := range m.batchTargets {
		sites[m.siteName(d.SiteID)]++
	}
	// Inventory order is stable; keep the preview predictable.
	seen := make(map[string]bool)
	for _, d := range m.batchTargets {
		name := m.siteName(d.SiteID)
		if !seen[name] && len(lines) < maxRows-2 {
			lines = append(lines, m.theme.muted().Render(fitText(fmt.Sprintf("%s: %d", m.privateField(name), sites[name]), innerW)))
			seen[name] = true
		}
	}
	targetSlots := max(1, maxRows-len(lines)-1)
	for i, d := range m.batchTargets {
		if i < m.batchPreviewIndex {
			continue
		}
		if i-m.batchPreviewIndex >= targetSlots {
			if len(lines) < maxRows-1 {
				lines = append(lines, m.theme.muted().Render(fitText("… remaining targets included in count", innerW)))
			}
			break
		}
		lines = append(lines, fitText("  "+m.privateField(d.Name)+" · "+m.privateField(m.siteName(d.SiteID)), innerW))
	}
	lines = append(lines, m.theme.muted().Render(fitText("j/k scroll · Enter confirm · Esc cancel", innerW)))
	if len(lines) > maxRows {
		lines = lines[:maxRows]
	}
	body := strings.Join(lines, "\n")
	if framed {
		return renderShearedBox(m.theme, m.theme.Glyph("diamond")+" BATCH PREVIEW", fmt.Sprintf("%d selected", len(m.batchTargets)), body, width, 0, true)
	}
	return body
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
	budget := m.bodyLineBudget()
	framed := width >= 44 && budget >= 8
	innerW := width
	maxRows := max(3, budget)
	if framed {
		innerW = width - 4
		maxRows = max(3, budget-2)
	}

	rows := []string{m.theme.muted().Render(fitText(fmt.Sprintf("Batch results %d/%d · j/k scroll · b back", len(m.batchResults), len(m.batchTargets)), innerW))}
	start := max(0, m.batchPreviewIndex)
	end := min(len(m.batchResults), start+max(1, maxRows-1))
	for _, r := range m.batchResults[start:end] {
		if len(rows) >= maxRows {
			break
		}
		statusStyled := stateStyle(m.theme, r.Status).Render(r.Status)
		rows = append(rows, fitText(m.privateField(r.Name)+" · "+statusStyled+" · "+m.privateField(r.Method), innerW))
		if r.Message != "" && len(rows) < maxRows {
			rows = append(rows, m.theme.muted().Render(fitText("  "+m.privateMessage(r.Message), innerW)))
		}
	}
	body := strings.Join(rows, "\n")
	if framed {
		return renderShearedBox(m.theme, m.theme.Glyph("diamond")+" BATCH RESULTS", fmt.Sprintf("%d/%d", len(m.batchResults), len(m.batchTargets)), body, width, 0, true)
	}
	return body
}
