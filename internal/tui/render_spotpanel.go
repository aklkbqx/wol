package tui

import (
	"fmt"
	"strings"

	"github.com/aklkbqx/wol/internal/store"
)

// Spotlight panels keep the machine pane on the left and draw the open
// panel on the stage: the key list, the action picker, the delete
// confirmation, batch preview and results, the first run and the remote
// launch. Each one uses the stage rows the machine view uses: tabs on
// row 1, a title on row 4, content from row 6, status at height-5 and
// keys at height-3.

const (
	panelHelp    = "help"
	panelPicker  = "picker"
	panelConfirm = "confirm"
	panelPreview = "preview"
	panelResults = "results"
	panelEmpty   = "empty"
	panelOpening = "opening"
)

// spotPanelKind names the Spotlight panel for the current state, or ""
// when the frame is the machine view, a form or the classic layout. The
// order follows handleKey, so the panel shown is the one keys act on.
func (m *WakeModel) spotPanelKind(width, height int) string {
	if !m.spotFrame(width, height).hasStage() {
		return ""
	}
	switch {
	case m.opening:
		return panelOpening
	case m.batchKind != "" && !m.batchRunning && len(m.batchTargets) > 0:
		return panelPreview
	case m.showBatchResults:
		return panelResults
	case m.form != nil, m.lanOpen:
		return ""
	case m.actionPicker:
		return panelPicker
	case m.confirm != "":
		if strings.HasPrefix(m.confirm, "machine:") {
			return panelConfirm
		}
		return ""
	case m.showHelp:
		return panelHelp
	case len(m.devices) == 0:
		return panelEmpty
	}
	return ""
}

// renderSpotPanel draws the full frame, width × height cells.
func (m *WakeModel) renderSpotPanel(kind string, width, height int) string {
	st := m.theme.spot()
	devices := m.filteredDevices()
	f := m.spotFrame(width, height)
	stageWidth := f.stage
	pane := m.spotPane(devices, f, st)
	var stage []string
	switch kind {
	case panelHelp:
		stage = m.spotHelpStage(stageWidth, height, st)
	case panelPicker:
		stage = m.spotPickerStage(devices, stageWidth, height, st)
	case panelConfirm:
		stage = m.spotConfirmStage(stageWidth, height, st)
	case panelPreview:
		stage = m.spotPreviewStage(stageWidth, height, st)
	case panelResults:
		stage = m.spotResultsStage(stageWidth, height, st)
	case panelOpening:
		stage = m.spotOpeningStage(stageWidth, height, st)
	default:
		stage = m.spotEmptyStage(stageWidth, height, st)
	}
	return spotCompose(pane, stage, f, st)
}

// spotHint is one key in the stage's key row.
type spotHint struct {
	key, label string
	primary    bool // amber key: the action enter or the key runs
	danger     bool // rose label: the action removes something
}

func spotBlankStage(width, height int, st spotStyles) []string {
	lines := make([]string, height)
	blank := st.stage.fill.pre + spaces(width) + st.stage.fill.post
	for i := range lines {
		lines[i] = blank
	}
	return lines
}

func (m *WakeModel) spotHintKey(row *spotRow, st spotStyles, h spotHint) {
	keyStyle, labelStyle := st.keycap, st.stage.soft
	if h.primary {
		keyStyle = st.button
	}
	if h.danger {
		labelStyle = st.stage.rose
	}
	row.add(keyStyle, " "+h.key+" ").add(st.stage.fill, " ").add(labelStyle, h.label)
}

// spotKeyRow lays out the keys left to right, most needed first, and
// pins right to the right edge. Keys that do not fit are dropped from
// the end.
func (m *WakeModel) spotKeyRow(width int, st spotStyles, hints []spotHint, right spotHint) string {
	s := st.stage
	keys := newSpotRow(width)
	rightW := 0
	if right.key != "" {
		rightW = spotKeyWidth(right.key, right.label) + 3
	}
	for i, h := range hints {
		gap := 0
		if i > 0 {
			gap = 3
		}
		if keys.used+gap+spotKeyWidth(h.key, h.label)+rightW > width {
			break
		}
		keys.add(s.fill, spaces(gap))
		m.spotHintKey(keys, st, h)
	}
	if right.key != "" {
		keys.to(s.fill, width-rightW+3)
		m.spotHintKey(keys, st, right)
	}
	return keys.String(s.fill)
}

// spotTitle is the panel title on row 4 with an optional note on the
// right of the measure.
func spotTitle(width int, st spotStyles, title, note string) string {
	s := st.stage
	measure := min(width, spotMeasure)
	row := newSpotRow(width)
	noteW := 0
	if note != "" {
		noteW = visibleWidth(note) + 3
	}
	row.add(s.bold, fitText(title, max(1, measure-noteW)))
	if note != "" && row.used+noteW <= measure {
		row.to(s.fill, measure-noteW+3).add(s.muted, note)
	}
	return row.String(s.fill)
}

// spotWrap splits text into lines of at most width cells at spaces.
func spotWrap(text string, width int) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case visibleWidth(line)+1+visibleWidth(word) <= width:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	for i := range out {
		out[i] = fitText(out[i], width)
	}
	return out
}

// spotParagraph writes text in tone from row, wrapped to the measure, and
// returns the row after it.
func spotParagraph(lines []string, row, width int, tone paint, st spotStyles, text string) int {
	for _, l := range spotWrap(text, min(width, spotMeasure)) {
		if row >= len(lines)-5 {
			break
		}
		lines[row] = newSpotRow(width).add(tone, l).String(st.stage.fill)
		row++
	}
	return row
}

// spotStatus writes the status line, rose when it needs attention.
func (m *WakeModel) spotStatus(lines []string, width, height int, st spotStyles) {
	s := st.stage
	msg := strings.TrimSpace(m.status)
	if msg == "" {
		return
	}
	style := s.muted
	if statusNeedsAttention(msg) {
		style = s.rose
	}
	lines[height-5] = newSpotRow(width).add(style, fitText(m.privateMessage(msg), width)).String(s.fill)
}

// spotHelpKeys lists every key, the selected machine first.
var spotHelpKeys = [2][][2]string{
	{
		{"enter", "pick an action"},
		{"w", "wake"},
		{"c", "wake and connect"},
		{"s  r", "check one, all"},
		{"P", "power off"},
		{"x", "end the stream"},
		{"p", "remote settings"},
		{"a e d", "add, edit, delete"},
	},
	{
		{"space", "mark a machine"},
		{"W  S", "wake, check marked"},
		{"R  b", "retry, see results"},
		{"n", "scan network"},
		{"/", "filter by name"},
		{"v [ ]", "cycle state, site"},
		{"tab", "next tab"},
		{"i  E", "import, export"},
		{"t", "next palette"},
		{"l", "list only, narrow window"},
	},
}

func (m *WakeModel) spotHelpStage(width, height int, st spotStyles) []string {
	s := st.stage
	lines := spotBlankStage(width, height, st)
	lines[1] = m.spotTabs(width, m.versionLabel(), false, st)
	lines[4] = spotTitle(width, st, "All keys", m.theme.PresetName())

	colW := min(34, (width-3)/2)
	keyW := 7
	heads := [2]string{"This machine", "Fleet and view"}
	rows := 0
	for _, col := range spotHelpKeys {
		rows = max(rows, len(col))
	}
	// Room for the headings, a blank row and every key.
	last := height - 7
	if 8+rows-1 > last || colW-keyW < 21 {
		return m.spotHelpPages(lines, width, height, st)
	}
	for r := -2; r < rows; r++ {
		y := 8 + r
		if r == -1 {
			continue
		}
		row := newSpotRow(width)
		for c, col := range spotHelpKeys {
			row.to(s.fill, c*(colW+3))
			switch {
			case r == -2:
				row.add(s.muted, heads[c])
			case r < len(col):
				row.add(s.text, col[r][0]).to(s.fill, c*(colW+3)+keyW)
				row.add(s.soft, fitText(col[r][1], colW-keyW))
			}
		}
		lines[y] = row.String(s.fill)
	}
	lines[height-3] = m.spotKeyRow(width, st, []spotHint{{key: "?", label: "close"}}, spotHint{key: "esc", label: "close"})
	return lines
}

// spotHelpEntries is the key list as one run: a heading, then its keys.
func spotHelpEntries() [][2]string {
	heads := [2]string{"This machine", "Fleet and view"}
	var out [][2]string
	for c, col := range spotHelpKeys {
		out = append(out, [2]string{"", heads[c]})
		out = append(out, col...)
	}
	return out
}

// spotHelpPages draws the key list in pages when the window is too short
// or too narrow for both columns at once. Space turns the page.
func (m *WakeModel) spotHelpPages(lines []string, width, height int, st spotStyles) []string {
	s := st.stage
	entries := spotHelpEntries()
	cols, colW := 1, min(width, 40)
	if (width-3)/2 >= 28 {
		cols, colW = 2, min(34, (width-3)/2)
	}
	const keyW = 7
	perCol := max(1, height-11) // rows 6..height-6
	perPage := perCol * cols
	pages := (len(entries) + perPage - 1) / perPage
	page := m.helpPage % pages
	chunk := entries[page*perPage : min(len(entries), (page+1)*perPage)]
	for r := 0; r < perCol; r++ {
		row := newSpotRow(width)
		for c := 0; c < cols; c++ {
			i := c*perCol + r
			if i >= len(chunk) {
				break
			}
			x := c * (colW + 3)
			row.to(s.fill, x)
			if e := chunk[i]; e[0] == "" {
				row.add(s.muted, e[1])
			} else {
				row.add(s.text, e[0]).to(s.fill, x+keyW).add(s.soft, fitText(e[1], colW-keyW))
			}
		}
		lines[6+r] = row.String(s.fill)
	}
	hints := []spotHint{{key: "?", label: "close"}}
	if pages > 1 {
		hints = []spotHint{{key: "space", label: fmt.Sprintf("more keys %d/%d", page+1, pages), primary: true}, {key: "?", label: "close"}}
	}
	lines[height-3] = m.spotKeyRow(width, st, hints, spotHint{key: "esc", label: "close"})
	return lines
}

// spotPickerItem is one choice in the action picker.
type spotPickerItem struct{ key, label, detail string }

func (m *WakeModel) spotPickerItems(device store.Device) []spotPickerItem {
	connect := spotPickerItem{"c", "Stream", "wake, then open the stream"}
	if profile, ok := m.profiles[device.ID]; ok && !streamProfile(profile) {
		connect = spotPickerItem{"c", "Remote", "wake, then open the remote"}
	}
	return []spotPickerItem{
		{"w", "Wake", "send the packet, then wait"},
		connect,
		{"s", "Check", "see if it is awake now"},
		{"P", "Power off", "open the shut-down form"},
		{"esc", "Cancel", "back to the machine"},
	}
}

func (m *WakeModel) spotPickerStage(devices []store.Device, width, height int, st spotStyles) []string {
	s := st.stage
	lines := spotBlankStage(width, height, st)
	lines[1] = m.spotTabs(width, m.spotFreshness(m.now()), m.stale, st)
	if len(devices) == 0 {
		lines[4] = spotTitle(width, st, "No machine selected", "")
		lines[height-3] = m.spotKeyRow(width, st, nil, spotHint{key: "esc", label: "close"})
		return lines
	}
	device := devices[min(m.selected, len(devices)-1)]
	lines[4] = spotTitle(width, st, m.privateField(device.Name), spotStateWord(m.deviceState(device)))

	items := m.spotPickerItems(device)
	measure := min(width, spotMeasure)
	step := 2
	if 6+len(items)*2-1 > height-6 {
		step = 1
	}
	labelCol, detailCol := 8, 21
	for i, it := range items {
		tone := s
		keyStyle, labelStyle := s.muted, s.soft
		if i == m.pickerSelected {
			tone = st.focus
			keyStyle, labelStyle = tone.amber, tone.bold
		}
		row := newSpotRow(width).add(tone.fill, "  ").add(keyStyle, it.key).to(tone.fill, labelCol)
		row.add(labelStyle, it.label).to(tone.fill, detailCol)
		row.add(tone.muted, fitText(it.detail, max(1, measure-detailCol-1))).to(tone.fill, measure)
		lines[6+i*step] = row.String(s.fill)
	}
	lines[height-3] = m.spotKeyRow(width, st, []spotHint{{key: "↑↓", label: "move"}, {key: "enter", label: "choose", primary: true}}, spotHint{key: "esc", label: "cancel"})
	return lines
}

func (m *WakeModel) spotConfirmStage(width, height int, st spotStyles) []string {
	s := st.stage
	lines := spotBlankStage(width, height, st)
	lines[1] = m.spotTabs(width, "", false, st)
	id := strings.TrimPrefix(m.confirm, "machine:")
	var device store.Device
	for _, d := range m.devices {
		if d.ID == id {
			device = d
			break
		}
	}
	name := m.privateField(device.Name)
	if strings.TrimSpace(name) == "" {
		name = "this machine"
	}
	lines[4] = newSpotRow(width).add(s.rose, "Delete ").add(s.bold, fitText(name, max(1, min(width, spotMeasure)-8))).add(s.bold, "?").String(s.fill)
	row := spotParagraph(lines, 6, width, s.soft, st, "It leaves this inventory with its remote and power settings. Nothing is sent to the machine.")

	if device.ID != "" && row+3 < height-5 {
		half := min(width, spotMeasure) / 2
		lab := newSpotRow(width).add(s.muted, "hardware").to(s.fill, half).add(s.muted, "site")
		val := newSpotRow(width).add(s.text, fitText(m.privateField(device.MACAddress), half-2)).to(s.fill, half)
		val.add(s.text, fitText(m.privateField(m.siteName(device.SiteID)), min(width, spotMeasure)-half))
		lines[row+1] = lab.String(s.fill)
		lines[row+2] = val.String(s.fill)
	}
	lines[height-3] = m.spotKeyRow(width, st, []spotHint{{key: "y", label: "delete", danger: true}}, spotHint{key: "esc", label: "keep it"})
	return lines
}

func (m *WakeModel) spotPreviewStage(width, height int, st spotStyles) []string {
	s := st.stage
	lines := spotBlankStage(width, height, st)
	lines[1] = m.spotTabs(width, "", false, st)
	n := len(m.batchTargets)
	verb, what := "Check", "Asks each one whether it is awake, 16 at a time."
	if m.batchKind == "wake" {
		verb, what = "Wake", "Sends a wake packet to each one, 16 at a time. Check them once they start."
	}
	sites := make(map[string]bool)
	for _, d := range m.batchTargets {
		sites[d.SiteID] = true
	}
	siteNote := "1 site"
	if len(sites) != 1 {
		siteNote = fmt.Sprintf("%d sites", len(sites))
	}
	lines[4] = spotTitle(width, st, fmt.Sprintf("%s %d %s", verb, n, plural(n, "machine", "machines")), fmt.Sprintf("%d selected · %s", n, siteNote))
	top := spotParagraph(lines, 6, width, s.muted, st, what) + 1

	measure := min(width, spotMeasure)
	siteCol := min(30, measure/2+2)
	bottom := height - 7
	visible := max(1, bottom-top+1)
	if n > visible {
		visible = max(1, visible-1) // keep a row for "↓ more"
	}
	start := min(max(0, m.batchPreviewIndex), max(0, n-visible))
	for k := 0; k < visible && start+k < n; k++ {
		d := m.batchTargets[start+k]
		glyph, tone := m.spotGlyph(d, s)
		row := newSpotRow(width).add(tone, glyph).add(s.fill, " ")
		row.add(s.text, fitText(m.privateField(d.Name), siteCol-3)).to(s.fill, siteCol)
		row.add(s.muted, fitText(m.privateField(m.siteName(d.SiteID)), measure-siteCol))
		lines[top+k] = row.String(s.fill)
	}
	if more := n - start - visible; more > 0 {
		lines[top+visible] = newSpotRow(width).add(s.muted, fmt.Sprintf("↓ %d more", more)).String(s.fill)
	} else if start > 0 {
		lines[min(top+visible, bottom)] = newSpotRow(width).add(s.muted, fmt.Sprintf("↑ %d above", start)).String(s.fill)
	}
	hints := []spotHint{{key: "enter", label: strings.ToLower(verb) + fmt.Sprintf(" %d", n), primary: true}}
	if n > visible {
		hints = append(hints, spotHint{key: "↑↓", label: "scroll"})
	}
	lines[height-3] = m.spotKeyRow(width, st, hints, spotHint{key: "esc", label: "cancel"})
	return lines
}

// spotResultWord groups a batch result status for the summary line.
func spotResultWord(status string) string {
	switch strings.ToUpper(status) {
	case "SENT":
		return "sent"
	case "ONLINE", "REACHABLE":
		return "awake"
	case "OFFLINE":
		return "asleep"
	case "CANCELLED":
		return "cancelled"
	case "UNKNOWN", "":
		return "unknown"
	}
	return "failed"
}

// spotResultMark returns the glyph and tone for a batch result status.
func spotResultMark(status string, s spotTone) (string, paint) {
	switch strings.ToUpper(status) {
	case "ONLINE", "SENT", "REACHABLE":
		return "●", s.green
	case "OFFLINE":
		return "○", s.muted
	case "CANCELLED", "UNKNOWN", "":
		return "◌", s.muted
	default:
		return "✕", s.rose
	}
}

func (m *WakeModel) spotResultsStage(width, height int, st spotStyles) []string {
	s := st.stage
	lines := spotBlankStage(width, height, st)
	lines[1] = m.spotTabs(width, "", false, st)
	verb := "Check"
	if m.lastBatchKind == "wake" {
		verb = "Wake"
	}
	keys := []spotHint{}
	right := spotHint{key: "esc", label: "close"}
	if len(m.batchResults) == 0 {
		lines[4] = spotTitle(width, st, "No batch results yet", "")
		if m.batchRunning {
			lines[4] = spotTitle(width, st, verb+" results", fmt.Sprintf("running 0 of %d", len(m.batchTargets)))
		} else {
			spotParagraph(lines, 6, width, s.soft, st, "Mark machines with space, then press W to wake them or S to check them. The results land here.")
		}
		lines[height-3] = m.spotKeyRow(width, st, keys, right)
		return lines
	}
	n := len(m.batchResults)
	note := fmt.Sprintf("%d of %d", n, len(m.batchTargets))
	if m.batchRunning {
		note = "running " + note
	}
	lines[4] = spotTitle(width, st, verb+" results", note)

	measure := min(width, spotMeasure)
	statusCol := min(26, measure/2)
	msgCol := statusCol + 11
	top, bottom := 6, height-7
	visible := max(1, bottom-top+1)
	if n > visible {
		visible = max(1, visible-1)
	}
	start := min(max(0, m.batchPreviewIndex), max(0, n-visible))
	for k := 0; k < visible && start+k < n; k++ {
		r := m.batchResults[start+k]
		glyph, tone := spotResultMark(r.Status, s)
		row := newSpotRow(width).add(tone, glyph).add(s.fill, " ")
		row.add(s.text, fitText(m.privateField(r.Name), statusCol-3)).to(s.fill, statusCol)
		row.add(tone, fitText(r.Status, 10)).to(s.fill, msgCol)
		if msg := strings.TrimSpace(m.privateMessage(r.Message)); msg != "" && measure > msgCol+4 {
			row.add(s.muted, fitText(msg, measure-msgCol))
		}
		lines[top+k] = row.String(s.fill)
	}
	if more := n - start - visible; more > 0 {
		lines[top+visible] = newSpotRow(width).add(s.muted, fmt.Sprintf("↓ %d more", more)).String(s.fill)
	}

	// Counts in a fixed order: what worked, what failed, the rest.
	counts := make(map[string]int)
	for _, r := range m.batchResults {
		counts[spotResultWord(r.Status)]++
	}
	sum := newSpotRow(width)
	for _, w := range []string{"sent", "awake", "failed", "asleep", "cancelled", "unknown"} {
		c := counts[w]
		if c == 0 {
			continue
		}
		_, tone := spotResultMark(w, s)
		if sum.used > 0 {
			sum.add(s.fill, "   ")
		}
		sum.add(tone, fmt.Sprintf("%d", c)).add(s.muted, " "+w)
	}
	lines[height-5] = sum.String(s.fill)
	if n > visible {
		keys = append(keys, spotHint{key: "↑↓", label: "scroll"})
	}
	lines[height-3] = m.spotKeyRow(width, st, keys, right)
	return lines
}

func (m *WakeModel) spotEmptyStage(width, height int, st spotStyles) []string {
	s := st.stage
	lines := spotBlankStage(width, height, st)
	lines[1] = m.spotTabs(width, "", false, st)
	lines[4] = spotTitle(width, st, "No machines yet", "")
	row := spotParagraph(lines, 6, width, s.soft, st, "Add one by its hardware address, look for machines on this network, or import an inventory you exported before.")
	act := newSpotRow(width).add(st.button, "  a   Add a machine  ")
	m.spotKeyFit(act, st, "n", "scan network")
	m.spotKeyFit(act, st, "i", "import")
	if row+1 < height-5 {
		lines[row+1] = act.String(s.fill)
	}
	m.spotStatus(lines, width, height, st)
	lines[height-3] = m.spotKeyRow(width, st, []spotHint{{key: "?", label: "all keys"}}, spotHint{key: "q", label: "quit"})
	return lines
}

func (m *WakeModel) spotOpeningStage(width, height int, st spotStyles) []string {
	s := st.stage
	lines := spotBlankStage(width, height, st)
	name := m.privateField(m.actionTarget)
	if strings.TrimSpace(name) == "" {
		name = "the machine"
	}
	last, lastDetail := "Remote", "open the remote session"
	if profile, ok := m.profiles[m.actionTargetID]; !ok || streamProfile(profile) {
		last, lastDetail = "Stream", "open the stream"
	}
	lines[1] = m.spotTabs(width, "wake and "+strings.ToLower(last), false, st)

	spin := []string{"◐", "◓", "◑", "◒"}[(int(m.frame)/3)%4]
	measure := min(width, spotMeasure)
	lines[4] = newSpotRow(width).add(s.amber, spin).add(s.fill, " ").add(s.bold, fitText("Connecting to "+name, measure-2)).String(s.fill)

	steps := [][2]string{
		{"Wake", "send the wake packet"},
		{"Wait", "until it answers on the network"},
		{last, lastDetail},
	}
	for i, step := range steps {
		row := newSpotRow(width).add(s.muted, fmt.Sprintf("%d", i+1)).to(s.fill, 4)
		row.add(s.text, step[0]).to(s.fill, 13).add(s.muted, fitText(step[1], max(1, measure-13)))
		lines[6+i*2] = row.String(s.fill)
	}
	spotParagraph(lines, 13, width, s.muted, st, "This can take a minute while it starts up.")
	if statusNeedsAttention(m.status) {
		m.spotStatus(lines, width, height, st)
	}
	lines[height-3] = m.spotKeyRow(width, st, []spotHint{{key: "q", label: "quit"}}, spotHint{key: "esc", label: "cancel"})
	return lines
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
