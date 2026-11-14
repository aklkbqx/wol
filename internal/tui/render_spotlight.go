package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/termimg"
)

// Spotlight is the Machines layout: a quiet machine list on the left and
// one large stage for the focused machine on the right. It is drawn on a
// fixed cell grid so every row is exactly the terminal width. Smaller
// windows keep the same parts: a narrower list, then the stage alone, then
// the list alone (see spotFrame).
const (
	spotMinWidth    = 90 // full list pane
	spotMinHeight   = 24 // full stage rows and room for a picture
	spotNarrowWidth = 72 // narrow list pane
	spotSingleWidth = 50 // stage alone
	spotStageHeight = 20 // fewest rows that hold a stage
	spotListWidth   = 30 // list alone, below this the classic layout
	spotListHeight  = 10
	spotPaneWidth   = 32 // left pane, including its 3-column inner margin
	spotGutter      = 3  // blank columns between pane and stage
	spotMargin      = 3  // left margin inside the pane
	spotRightPad    = 2  // blank columns right of the stage
	spotMeasure     = 72 // widest the stage content grows
	spotStackWidth  = 60 // narrower stages list the facts in one column
	spotTallWidth   = 36 // portrait phone: picture above the facts
	spotTallHeight  = 30
	spotSideWidth   = 44 // list column beside the machine on a wide, short window
)

// Spotlight frame modes, widest first.
const (
	spotNone   = iota // classic layout
	spotFull          // 32-column pane and the stage
	spotNarrow        // 24-column pane and the stage
	spotSingle        // the stage alone, neighbours named below it
	spotList          // the list alone
)

// spotFrame is how a Spotlight frame splits its width.
type spotFrame struct {
	mode                 int
	pane, margin         int // pane width and the margin inside it, 0 without a pane
	gutter, right, stage int // columns left of, right of and inside the stage
	width, height        int
	tall                 bool // portrait: picture above the facts, tall buttons
}

// hasStage reports whether the frame draws the stage, so panels and forms
// can use it.
func (f spotFrame) hasStage() bool {
	return f.mode == spotFull || f.mode == spotNarrow || f.mode == spotSingle
}

// spotFrame picks the Spotlight mode for the window. Only the Machines tab
// uses it, and ASCII keeps the classic layout.
func (m *WakeModel) spotFrame(width, height int) spotFrame {
	f := spotFrame{width: width, height: height}
	if m.tab != 0 || m.theme.ASCII || width < spotListWidth || height < spotListHeight {
		return f
	}
	// A portrait window (a phone held upright) stacks the stage instead of
	// leaving rows empty below it. Cells are about twice as tall as wide, so
	// height*2 >= width means the window is at least square on screen.
	if !m.listView && width >= spotTallWidth && width < spotNarrowWidth && height >= spotTallHeight && height*2 >= width {
		f.mode, f.tall, f.gutter, f.right = spotSingle, true, 3, 3
		if width < spotSingleWidth {
			f.gutter, f.right = 2, 2
		}
		f.stage = width - f.gutter - f.right
		return f
	}
	switch {
	case width < spotSingleWidth || height < spotStageHeight:
		f.mode = spotList
	case width >= spotMinWidth:
		f.mode, f.pane, f.margin, f.gutter, f.right = spotFull, spotPaneWidth, spotMargin, spotGutter, spotRightPad
	case width >= spotNarrowWidth:
		f.mode, f.pane, f.margin, f.gutter, f.right = spotNarrow, 24, 2, 2, 2
	case m.listView:
		f.mode = spotList
	default:
		f.mode, f.gutter, f.right = spotSingle, 3, 3
	}
	if f.mode != spotList {
		f.stage = width - f.pane - f.gutter - f.right
	}
	return f
}

// wakeAnswerWindow is how long a woken machine has to answer its check.
const wakeAnswerWindow = 90 * time.Second

// paint is a style reduced to its escape-code prefix and suffix, so a row
// can be assembled by concatenation instead of one Render call per run.
type paint struct{ pre, post string }

func newPaint(st lipgloss.Style) paint {
	const marker = "\x01"
	out := st.Render(marker)
	i := strings.Index(out, marker)
	if i < 0 {
		return paint{}
	}
	return paint{pre: out[:i], post: out[i+len(marker):]}
}

// spotTone is one background with every foreground the Spotlight uses on it.
type spotTone struct {
	fill, text, bold, soft, muted, amber, green, rose paint
}

type spotStyles struct {
	pane, focus, stage spotTone
	keycap, button     paint
	track              paint
}

func toneOf(fill, text, bold, soft, muted, amber, green, rose lipgloss.Style) spotTone {
	return spotTone{
		fill: newPaint(fill), text: newPaint(text), bold: newPaint(bold), soft: newPaint(soft),
		muted: newPaint(muted), amber: newPaint(amber), green: newPaint(green), rose: newPaint(rose),
	}
}

func buildSpotStyles(p Palette, colors bool) spotStyles {
	if !colors {
		plain := lipgloss.NewStyle()
		bold := plain.Bold(true)
		tone := toneOf(plain, plain, bold, plain, plain, bold, plain, plain)
		rev := plain.Reverse(true)
		focus := toneOf(rev, rev, rev.Bold(true), rev, rev, rev.Bold(true), rev, rev)
		return spotStyles{pane: tone, focus: focus, stage: tone, keycap: newPaint(rev.Bold(true)), button: newPaint(rev.Bold(true)), track: newPaint(plain)}
	}
	tone := func(bg color.Color) spotTone {
		base := lipgloss.NewStyle().Background(bg)
		return toneOf(base, base.Foreground(p.Text), base.Foreground(p.Text).Bold(true), base.Foreground(p.Slate),
			base.Foreground(p.Muted), base.Foreground(p.Amber).Bold(true), base.Foreground(p.Emerald), base.Foreground(p.Rose))
	}
	track := p.Track
	if track == nil {
		track = p.CardLine
	}
	return spotStyles{
		pane:   tone(p.Panel),
		focus:  tone(p.PanelHot),
		stage:  tone(p.Ink),
		keycap: newPaint(lipgloss.NewStyle().Foreground(p.Text).Background(p.CardLine).Bold(true)),
		button: newPaint(lipgloss.NewStyle().Foreground(p.Ink).Background(p.Amber).Bold(true)),
		track:  newPaint(lipgloss.NewStyle().Foreground(track).Background(p.Ink)),
	}
}

func (t Theme) spot() spotStyles { return t.ensureStyles().spot }

// spotRow builds one fixed-width run of styled cells.
type spotRow struct {
	b    strings.Builder
	used int
	max  int
}

func newSpotRow(width int) *spotRow { return &spotRow{max: width} }

// blankRun holds enough spaces for any padding run, so padding never
// allocates.
var blankRun = strings.Repeat(" ", 512)

func spaces(n int) string {
	if n <= len(blankRun) {
		return blankRun[:n]
	}
	return strings.Repeat(" ", n)
}

// add writes plain text in a style, clipping at the row width.
func (r *spotRow) add(style paint, text string) *spotRow {
	if text == "" || r.used >= r.max {
		return r
	}
	room := r.max - r.used
	w := visibleWidth(text)
	if w > room {
		text = visiblePrefix([]rune(text), room)
		w = visibleWidth(text)
	}
	r.b.WriteString(style.pre)
	r.b.WriteString(text)
	r.b.WriteString(style.post)
	r.used += w
	return r
}

// to pads with the fill style up to column col.
func (r *spotRow) to(fill paint, col int) *spotRow {
	if col > r.max {
		col = r.max
	}
	if col > r.used {
		r.b.WriteString(fill.pre)
		r.b.WriteString(spaces(col - r.used))
		r.b.WriteString(fill.post)
		r.used = col
	}
	return r
}

// raw writes cells that are already styled and exactly w columns wide,
// such as a picture line. It is skipped when it does not fit.
func (r *spotRow) raw(cells string, w int) *spotRow {
	if cells == "" || r.used+w > r.max {
		return r
	}
	r.b.WriteString(cells)
	r.used += w
	return r
}

func (r *spotRow) String(fill paint) string {
	r.to(fill, r.max)
	return r.b.String()
}

func (m *WakeModel) spotlightEligible(width, height int) bool {
	f := m.spotFrame(width, height)
	return f.mode != spotNone && (len(m.devices) > 0 || f.mode == spotList) &&
		m.form == nil && !m.showBatchResults && (m.batchKind == "" || m.batchRunning) &&
		!m.lanOpen && !m.actionPicker && !m.showHelp && m.confirm == ""
}

// renderSpotlight draws the full frame, width × height cells.
func (m *WakeModel) renderSpotlight(width, height int) string {
	st := m.theme.spot()
	devices := m.filteredDevices()
	f := m.spotFrame(width, height)
	if f.mode == spotList {
		return m.renderSpotList(devices, f, st)
	}
	if f.tall {
		return spotCompose(nil, m.spotTallStage(devices, f, st), f, st)
	}
	stage := m.spotStage(devices, f.stage, height, st)
	if f.mode == spotSingle {
		m.spotSingleChrome(stage, devices, f, st)
	}
	return spotCompose(m.spotPane(devices, f, st), stage, f, st)
}

// spotCompose joins the pane and the stage into the full frame. A frame
// without a pane passes nil.
func spotCompose(pane, stage []string, f spotFrame, st spotStyles) string {
	var out strings.Builder
	out.Grow(f.width * f.height * 4)
	gutter := st.stage.fill.pre + spaces(f.gutter) + st.stage.fill.post
	right := st.stage.fill.pre + spaces(f.right) + st.stage.fill.post
	for i := 0; i < f.height; i++ {
		if i > 0 {
			out.WriteByte('\n')
		}
		if pane != nil {
			out.WriteString(pane[i])
		}
		out.WriteString(gutter)
		out.WriteString(stage[i])
		out.WriteString(right)
	}
	return out.String()
}

// spotTabs is the stage header: the tab names and, on the right, a note
// such as the check freshness.
func (m *WakeModel) spotTabs(width int, note string, alert bool, st spotStyles) string {
	s := st.stage
	tabs := newSpotRow(width)
	gap := " "
	if width < 38 {
		gap = "" // a small phone: all four tabs still fit
	}
	for i, name := range []string{"Machines", "Routes", "Activity", "Sites"} {
		if i > 0 {
			tabs.add(s.fill, gap)
		}
		if i == m.tab {
			tabs.add(st.focus.bold, " "+name+" ")
		} else {
			tabs.add(s.muted, " "+name+" ")
		}
	}
	noteStyle := s.muted
	if alert {
		noteStyle = s.rose
	}
	if note != "" && tabs.used+3+visibleWidth(note) <= width {
		tabs.to(s.fill, width-visibleWidth(note)).add(noteStyle, note)
	}
	return tabs.String(s.fill)
}

func spotStateWord(state string) string {
	switch state {
	case "ONLINE":
		return "awake"
	case "OFFLINE":
		return "asleep"
	case "CHECKING":
		return "checking"
	case "DISABLED":
		return "disabled"
	default:
		return "unknown"
	}
}

// spotGlyph returns the list glyph and its tone colour for a machine.
func (m *WakeModel) spotGlyph(device store.Device, tone spotTone) (string, paint) {
	if m.waking && device.ID == m.actionTargetID {
		return "◐", tone.amber
	}
	switch m.deviceState(device) {
	case "ONLINE":
		return "●", tone.green
	case "OFFLINE":
		return "○", tone.muted
	case "DISABLED", "BLOCKED", "FAILED":
		return "✕", tone.rose
	default:
		return "◌", tone.muted
	}
}

func (m *WakeModel) spotPane(devices []store.Device, f spotFrame, st spotStyles) []string {
	if f.pane == 0 {
		return nil
	}
	height, paneWidth, margin := f.height, f.pane, f.margin
	lines := make([]string, height)
	p := st.pane
	blank := p.fill.pre + spaces(paneWidth) + p.fill.post
	for i := range lines {
		lines[i] = blank
	}
	inner := paneWidth - margin - 2

	site := "all sites"
	if m.siteFilter != "" {
		site = m.privateField(m.siteName(m.siteFilter))
	}
	head := newSpotRow(paneWidth).to(p.fill, margin)
	if logo := m.paneLogo(); logo != "" {
		head.raw(logo, 2)
	} else {
		head.add(p.amber, "●")
	}
	head.add(p.fill, " ").add(p.bold, "wol").add(p.fill, "  ")
	head.add(p.soft, fitText(site, paneWidth-head.used-2))
	lines[1] = head.String(p.fill)

	count := fmt.Sprintf("%d", len(devices))
	label := newSpotRow(paneWidth).to(p.fill, margin).add(p.muted, "MACHINES")
	label.to(p.fill, paneWidth-2-len(count)).add(p.muted, count)
	lines[4] = label.String(p.fill)

	// Two rows per machine. Rows 6..height-5 hold the list.
	first, last := 6, height-5
	slots := max(1, (last-first+1)/2)
	start := 0
	if m.selected >= slots {
		start = m.selected - slots + 1
	}
	for i := 0; i < slots && start+i < len(devices); i++ {
		idx := start + i
		device := devices[idx]
		tone := p
		focused := idx == m.selected
		if focused {
			tone = st.focus
		}
		glyph, gStyle := m.spotGlyph(device, tone)
		nameStyle := tone.soft
		if focused {
			nameStyle = tone.bold
		}
		row := newSpotRow(paneWidth).to(p.fill, margin-1)
		row.add(tone.fill, " ")
		if icon := m.paneIcon(device, focused); icon != "" {
			row.raw(icon, 2)
		} else {
			row.add(gStyle, glyph)
		}
		row.add(tone.fill, " ")
		row.add(nameStyle, fitText(m.privateField(device.Name), paneWidth-row.used-4))
		if m.marked[device.ID] {
			row.to(tone.fill, paneWidth-4).add(tone.amber, "✓")
		}
		row.to(tone.fill, paneWidth-2)
		lines[first+i*2] = row.String(p.fill)
	}
	if len(devices) == 0 {
		empty := "nothing matches"
		if len(m.devices) == 0 {
			empty = "no machines yet"
		}
		lines[first] = newSpotRow(paneWidth).to(p.fill, margin).add(p.muted, empty).String(p.fill)
	} else if start+slots < len(devices) {
		more := fmt.Sprintf("+%d more", len(devices)-start-slots)
		lines[last+1] = newSpotRow(paneWidth).to(p.fill, margin).add(p.muted, more).String(p.fill)
	}

	if m.filtering {
		row := newSpotRow(paneWidth).to(p.fill, margin).add(p.amber, "/ ").add(p.text, fitText(m.privateField(m.filterInput), inner-3)).add(p.amber, "_")
		lines[height-4] = row.String(p.fill)
	} else if chips := m.spotFilterChips(); chips != "" {
		lines[height-4] = newSpotRow(paneWidth).to(p.fill, margin).add(p.amber, fitText(chips, inner)).String(p.fill)
	}

	online, offline, _ := m.machineSummary(devices)
	sum := newSpotRow(paneWidth).to(p.fill, margin)
	sum.add(p.soft, fmt.Sprintf("%d", online)).add(p.muted, " awake  ").add(p.soft, fmt.Sprintf("%d", offline)).add(p.muted, " asleep")
	if n := len(m.marked); n > 0 {
		if marked := fmt.Sprintf("%d marked", n); sum.used+2+len(marked) <= paneWidth-2 {
			sum.add(p.muted, "  ").add(p.amber, marked)
		}
	}
	lines[height-3] = sum.String(p.fill)
	return lines
}

func (m *WakeModel) spotFilterChips() string {
	var chips []string
	if m.stateFilter != "" {
		chips = append(chips, "state "+strings.ToLower(m.stateFilter))
	}
	if m.filter != "" {
		chips = append(chips, "/"+m.privateField(m.filter))
	}
	return strings.Join(chips, "  ")
}

func (m *WakeModel) spotFreshness(now time.Time) string {
	if m.checkedAt.IsZero() {
		return "not checked yet"
	}
	if m.stale {
		return "stale, checked " + m.checkedAt.Format("15:04")
	}
	age := now.Sub(m.checkedAt)
	switch {
	case age < 5*time.Second:
		return "checked just now"
	case age < time.Minute:
		return fmt.Sprintf("checked %d s ago", int(age.Seconds()))
	case age < time.Hour:
		return fmt.Sprintf("checked %d min ago", int(age.Minutes()))
	default:
		return "checked " + m.checkedAt.Format("15:04")
	}
}

func (m *WakeModel) spotStage(devices []store.Device, width, height int, st spotStyles) []string {
	s := st.stage
	lines := make([]string, height)
	blank := s.fill.pre + spaces(width) + s.fill.post
	for i := range lines {
		lines[i] = blank
	}
	now := m.now()

	lines[1] = m.spotTabs(width, m.spotFreshness(now), m.stale, st)

	if len(devices) == 0 {
		lines[6] = newSpotRow(width).add(s.text, "No machine matches this filter.").String(s.fill)
		lines[8] = newSpotRow(width).add(s.muted, "Press / to change it, or v and [ ] to cycle state and site.").String(s.fill)
		m.spotFooter(lines, width, height, st, false)
		return lines
	}
	device := devices[min(m.selected, len(devices)-1)]
	state := m.deviceState(device)
	waking := m.waking && device.ID == m.actionTargetID
	name := m.privateField(device.Name)
	lay := m.spotLayout(width, height)

	// rowAt starts stage row i: picture cells first on the rows the
	// picture covers, so text there begins at the text column.
	var pic []string
	var photo bool
	var note string
	if lay.art {
		pic, photo, note = m.stagePicture(device, lay.artCols, lay.artRows, now)
	}
	rowAt := func(i int) *spotRow {
		row := newSpotRow(width)
		if j := i - lay.artTop; lay.art && j >= 0 && j < lay.artRows && j < len(pic) {
			row.raw(pic[j], lay.artCols).to(s.fill, lay.textCol)
		}
		return row
	}

	if lay.art {
		for i := 0; i < lay.artRows; i++ {
			lines[lay.artTop+i] = rowAt(lay.artTop + i).String(s.fill)
		}
		big, bigOK := blockText(name, (width-lay.textCol+1)/4)
		for i := 0; i < 3; i++ {
			row := rowAt(lay.nameRow + i)
			if bigOK {
				row.add(s.text, big[i])
			} else if i == 1 {
				row.add(s.bold, fitText(name, width-lay.textCol))
			}
			lines[lay.nameRow+i] = row.String(s.fill)
		}
		kind, _ := m.artKind(device)
		typeRow := rowAt(lay.typeRow).add(s.muted, kind.Label())
		if photo {
			typeRow.add(s.muted, "  ·  your picture")
		} else if note != "" {
			typeRow.add(s.muted, "  ·  "+note)
		}
		lines[lay.typeRow] = typeRow.String(s.fill)
		if waking {
			m.spotWaking(lines, device, width, now, st, lay, rowAt)
			m.spotFooter(lines, width, height, st, true)
			return lines
		}
		m.spotDetails(lines, device, state, name, width, st, lay, rowAt)
		m.spotFooter(lines, width, height, st, false)
		return lines
	}

	// Lamp (6 cols) and the machine name in block letters.
	lampStyle := s.muted
	switch {
	case waking:
		lampStyle = s.amber
	case state == "ONLINE":
		lampStyle = s.green
	case state == "DISABLED":
		lampStyle = s.rose
	}
	lamp := []string{"▄████▄", "██████", "▀████▀"}
	big, bigOK := blockText(name, (width-9)/4)
	for i := 0; i < 3; i++ {
		row := newSpotRow(width).add(lampStyle, lamp[i]).add(s.fill, "   ")
		if bigOK {
			row.add(s.text, big[i])
		} else if i == 1 {
			row.add(s.bold, fitText(name, width-9))
		}
		lines[lay.nameRow+i] = row.String(s.fill)
	}

	if waking {
		m.spotWaking(lines, device, width, now, st, lay, rowAt)
		m.spotFooter(lines, width, height, st, true)
		return lines
	}
	m.spotDetails(lines, device, state, name, width, st, lay, rowAt)
	m.spotFooter(lines, width, height, st, false)
	return lines
}

// spotDetails writes the state line, the facts and the primary action.
func (m *WakeModel) spotDetails(lines []string, device store.Device, state, name string, width int, st spotStyles, lay spotLayout, rowAt func(int) *spotRow) {
	s := st.stage
	// State line.
	word := spotStateWord(state)
	wordStyle := s.soft
	if state == "ONLINE" {
		wordStyle = s.green
	} else if state == "DISABLED" {
		wordStyle = s.rose
	}
	detail := m.spotStateDetail(device, state)
	stateRow := rowAt(lay.stateRow).add(wordStyle, word)
	if lay.art && stateRow.used+3+visibleWidth(detail) > width {
		lines[lay.stateRow] = stateRow.String(s.fill)
		lines[lay.stateRow+1] = rowAt(lay.stateRow+1).add(s.muted, fitText(detail, width-lay.textCol)).String(s.fill)
	} else {
		stateRow.add(s.fill, "   ")
		lines[lay.stateRow] = stateRow.add(s.muted, fitText(detail, width-stateRow.used)).String(s.fill)
	}

	// Facts, two columns, kept to a readable measure on wide terminals.
	half := min(width, spotMeasure) / 2
	addr := strings.TrimSpace(device.IPAddress)
	if addr == "" {
		addr = "no address"
	}
	checks := "port sweep and ping"
	if port := m.verifyPort(device); port > 0 {
		checks = fmt.Sprintf("tcp:%d", port)
	}
	facts := [][2][2]string{
		{{"address", m.privateField(addr)}, {"hardware", m.privateField(device.MACAddress)}},
		{{"wakes by", m.privateMessage(m.routeText(device))}, {"checks", checks}},
	}
	if width < spotStackWidth {
		for i, fact := range [][2]string{facts[0][0], facts[0][1], facts[1][0], facts[1][1]} {
			row := newSpotRow(width).add(s.muted, fact[0]).to(s.fill, 10).add(s.text, fitText(fact[1], width-10))
			lines[lay.factsRow+i] = row.String(s.fill)
		}
	}
	for i, pair := range facts {
		if width < spotStackWidth {
			break
		}
		lab := newSpotRow(width).add(s.muted, pair[0][0]).to(s.fill, half).add(s.muted, pair[1][0])
		val := newSpotRow(width).add(s.text, fitText(pair[0][1], half-2)).to(s.fill, half).add(s.text, fitText(pair[1][1], min(width, spotMeasure)-half))
		lines[lay.factsRow+i*2] = lab.String(s.fill)
		lines[lay.factsRow+1+i*2] = val.String(s.fill)
	}

	// Primary action.
	act := newSpotRow(width)
	switch {
	case m.opening || m.shuttingDown:
		act.add(s.amber, fitText(m.privateMessage(m.status), width))
	case !m.isWakeReady(device) && state != "ONLINE":
		why := m.wakeCapability(device).detail
		act.add(s.rose, "✕ can't wake  ").add(s.muted, fitText(m.privateMessage(why), width-24)).add(s.fill, "   ")
		m.spotKey(act, st, "e", "edit")
	case state == "ONLINE":
		if _, ok := m.profiles[device.ID]; ok {
			act.add(st.button, "  c   Connect "+fitText(name, 20)+"  ")
		} else {
			act.add(st.button, "  s   Check "+fitText(name, 20)+"  ")
		}
		if _, ok := m.profiles[device.ID]; ok {
			m.spotKeyFit(act, st, "s", "check")
		} else {
			m.spotKeyFit(act, st, "p", "set up remote")
		}
		m.spotKeyFit(act, st, "P", "shut down")
	default:
		act.add(st.button, "  w   Wake "+fitText(name, 20)+"  ")
		m.spotKeyFit(act, st, "c", "wake and connect")
		m.spotKeyFit(act, st, "s", "check")
	}
	lines[lay.actionRow] = act.String(s.fill)
}

func (m *WakeModel) verifyPort(device store.Device) int {
	if device.VerifyPort > 0 {
		return device.VerifyPort
	}
	if profile, ok := m.profiles[device.ID]; ok && profile.VerifyPort > 0 {
		return profile.VerifyPort
	}
	return 0
}

func (m *WakeModel) spotStateDetail(device store.Device, state string) string {
	method := strings.TrimSpace(m.presenceMethod[device.ID])
	at, seen := m.checkedDevice[device.ID]
	switch state {
	case "ONLINE":
		if how := m.spotMethod(device, method); how != "" {
			return "answered " + how
		}
		return "answering"
	case "OFFLINE":
		if seen {
			return "no answer at " + at.Format("15:04")
		}
		return "no answer on the last check"
	case "DISABLED":
		return "turned off in the inventory"
	case "CHECKING":
		return "checking now"
	}
	return "not checked yet, press s"
}

// spotMethod turns a presence method into words.
func (m *WakeModel) spotMethod(device store.Device, method string) string {
	switch method {
	case "tcp_verify", "tcp_refused":
		if port := m.verifyPort(device); port > 0 {
			return fmt.Sprintf("on tcp:%d", port)
		}
		return "on its check port"
	case "tcp_sweep":
		return "on a common port"
	case "icmp":
		return "to ping"
	case "arp":
		return "on the local network (arp)"
	case "", "none":
		return ""
	default:
		return "by " + m.privateField(method)
	}
}

func (m *WakeModel) spotWaking(lines []string, device store.Device, width int, now time.Time, st spotStyles, lay spotLayout, rowAt func(int) *spotRow) {
	s := st.stage
	waitFor := "waiting for an answer"
	if port := m.verifyPort(device); port > 0 {
		waitFor = fmt.Sprintf("waiting for tcp:%d to answer", port)
	}
	sent := m.action == "wake-wait" && !m.wakeSentAt.IsZero()
	detail := "sending the wake packet"
	if sent {
		detail = fmt.Sprintf("packet sent %d s ago", int(now.Sub(m.wakeSentAt).Seconds()))
	}
	head := rowAt(lay.stateRow).add(s.amber, "waking").add(s.fill, "   ")
	lines[lay.stateRow] = head.add(s.muted, fitText(detail, width-head.used)).String(s.fill)

	left := wakeAnswerWindow
	if sent {
		left = wakeAnswerWindow - now.Sub(m.wakeSentAt)
	}
	if left < 0 {
		left = 0
	}
	secs := int(left.Round(time.Second).Seconds())
	clock := fmt.Sprintf("%d:%02d", secs/60, secs%60)
	caption := fmt.Sprintf("seconds left of %d", int(wakeAnswerWindow.Seconds()))
	barRow := lay.factsRow + 4
	if lay.plain {
		lines[lay.factsRow] = newSpotRow(width).add(s.bold, clock).add(s.fill, "   ").add(s.muted, fitText(caption, width-len(clock)-3)).String(s.fill)
		barRow = lay.factsRow + 2
	} else {
		big, _ := blockText(clock, 8)
		for i := 0; i < 3; i++ {
			row := newSpotRow(width).add(s.text, big[i])
			if i == 1 {
				row.add(s.fill, "   ").add(s.muted, caption)
			}
			lines[lay.factsRow+i] = row.String(s.fill)
		}
	}

	barW := min(width, spotMeasure)
	done := 0
	if sent {
		done = int(float64(barW) * float64(wakeAnswerWindow-left) / float64(wakeAnswerWindow))
	}
	bar := newSpotRow(width).add(s.amber, strings.Repeat("▄", done)).add(st.track, strings.Repeat("▄", barW-done))
	lines[barRow] = bar.String(s.fill)

	route := m.privateMessage(m.routeText(device))
	if sent {
		lines[lay.actionRow] = newSpotRow(width).add(s.green, "✓").add(s.fill, " ").add(s.soft, fitText("sent by "+route, width-2)).String(s.fill)
		lines[lay.actionRow+1] = newSpotRow(width).add(s.amber, "◐").add(s.fill, " ").add(s.soft, fitText(waitFor, width-2)).String(s.fill)
	} else {
		lines[lay.actionRow] = newSpotRow(width).add(s.amber, "◐").add(s.fill, " ").add(s.soft, fitText("sending by "+route, width-2)).String(s.fill)
	}
}

// spotKeyFit adds a key hint after a 3-column gap only when the whole hint
// fits in the row.
func (m *WakeModel) spotKeyFit(row *spotRow, st spotStyles, key, label string) {
	if row.used+3+spotKeyWidth(key, label) > row.max {
		return
	}
	row.add(st.stage.fill, "   ")
	m.spotKey(row, st, key, label)
}

func spotKeyWidth(key, label string) int {
	return visibleWidth(key) + 3 + visibleWidth(label)
}

func (m *WakeModel) spotKey(row *spotRow, st spotStyles, key, label string) {
	row.add(st.keycap, " "+key+" ").add(st.stage.fill, " ").add(st.stage.soft, label)
}

// spotFooter writes the status line and the key hints at the bottom.
func (m *WakeModel) spotFooter(lines []string, width, height int, st spotStyles, busy bool) {
	s := st.stage
	if msg := strings.TrimSpace(m.status); msg != "" && !busy {
		style := s.muted
		if statusNeedsAttention(msg) {
			style = s.rose
		}
		lines[height-5] = newSpotRow(width).add(style, fitText(m.privateMessage(msg), width)).String(s.fill)
	}
	keys := newSpotRow(width)
	hints := [][2]string{{"↑↓", "move"}, {"space", "mark"}, {"/", "filter"}, {"?", "all keys"}}
	if busy {
		hints = [][2]string{{"esc", "cancel"}, {"?", "all keys"}}
	}
	quit := spotKeyWidth("q", "quit")
	for i, h := range hints {
		gap := 0
		if i > 0 {
			gap = 3
		}
		if keys.used+gap+spotKeyWidth(h[0], h[1])+3+quit > width {
			break
		}
		keys.add(s.fill, spaces(gap))
		m.spotKey(keys, st, h[0], h[1])
	}
	keys.to(s.fill, width-quit)
	m.spotKey(keys, st, "q", "quit")
	lines[height-3] = keys.String(s.fill)
}

// spotLayout places the stage blocks. Without pictures it is the fixed
// lamp layout; with pictures the room or photo sits at the top left, the
// name, state and type sit beside it, and the facts follow below.
type spotLayout struct {
	art                      bool
	artTop, artRows, artCols int
	textCol                  int
	nameRow, stateRow        int
	typeRow                  int
	factsRow, actionRow      int
	plain                    bool // no block letters: phone line spacing splits them
}

const (
	spotArtMinRows = 6  // smaller pictures do not read; use the lamp
	spotArtMaxRows = 16 // larger pictures crowd the facts
	spotArtTextMin = 37 // columns kept beside the picture: a 9-letter block name
)

func (m *WakeModel) spotLayout(width, height int) spotLayout {
	if height < spotMinHeight {
		// Short window: the blocks move up and the picture gives way.
		return spotLayout{nameRow: 3, stateRow: 7, factsRow: 9, actionRow: 14}
	}
	lay := spotLayout{nameRow: 5, stateRow: 9, factsRow: 12, actionRow: 18}
	if !m.artOn() {
		return lay
	}
	// Tallest picture that still leaves the facts and the action above the
	// status line, and the name column beside it.
	rows := min(height-16, spotArtMaxRows)
	rows = min(min(rows, (width-spotArtTextMin)/3), termimg.MaxPlaceholder)
	if rows < spotArtMinRows {
		return lay
	}
	lay.art = true
	lay.artTop, lay.artRows, lay.artCols = 3, rows, rows*3
	lay.textCol = lay.artCols + 3
	// Name (3 rows), gap, state, gap, type: centred beside the picture.
	top := lay.artTop + max(0, (rows-7)/2)
	lay.nameRow, lay.stateRow, lay.typeRow = top, top+4, top+6
	lay.factsRow = lay.artTop + max(rows, 8) + 1
	lay.actionRow = lay.factsRow + 6
	return lay
}

// paneLogo is the power logo for the pane header, or "" to use a glyph.
func (m *WakeModel) paneLogo() string {
	if !m.artKitty() {
		return ""
	}
	return m.logoMark(m.theme.Palette.Panel)
}

// paneIcon is the machine icon for a list row, or "" to use a glyph.
func (m *WakeModel) paneIcon(device store.Device, focused bool) string {
	if !m.artKitty() {
		return ""
	}
	bg := m.theme.Palette.Panel
	if focused {
		bg = m.theme.Palette.PanelHot
	}
	return m.listIcon(device, bg)
}

// blockFont is a 3×5 pixel font; '#' is lit. Letters J and Q are absent
// because they do not read at this size; names using them fall back to text.
var blockFont = map[rune]string{
	'A': ".#.#.#####.##.#", 'B': "##.#.###.#.###.", 'C': ".###..#..#...##", 'D': "##.#.##.##.###.",
	'E': "####..##.#..###", 'F': "####..##.#..#..", 'G': ".###..#.##.#.##", 'H': "#.##.#####.##.#",
	'I': "###.#..#..#.###", 'K': "#.##.###.#.##.#", 'L': "#..#..#..#..###", 'M': "#.########.##.#",
	'N': "##.#.##.##.##.#", 'O': ".#.#.##.##.#.#.", 'P': "##.#.###.#..#..", 'R': "##.#.###.#.##.#",
	'S': ".###...#...###.", 'T': "###.#..#..#..#.", 'U': "#.##.##.##.####", 'V': "#.##.##.##.#.#.",
	'W': "#.##.########.#", 'X': "#.##.#.#.#.##.#", 'Y': "#.##.#.#..#..#.", 'Z': "###..#.#.#..###",
	'0': "####.##.##.####", '1': ".#.##..#..#.###", '2': "##...#.#.#..###", '3': "##...#.#...###.",
	'4': "#.##.####..#..#", '5': "####..##...###.", '6': ".###..####.####", '7': "###..#.#..#..#.",
	'8': "####.#####.####", '9': "####.####..###.", '-': "......###......", ':': "....#.....#....",
	' ': "...............",
}

// blockText renders text in the block font as three rows of half-block
// characters. It reports false when a rune is missing or the text is longer
// than maxChars, so the caller can fall back to plain text.
func blockText(text string, maxChars int) ([3]string, bool) {
	var rows [3]string
	runes := []rune(strings.ToUpper(text))
	if len(runes) == 0 || len(runes) > maxChars {
		return rows, false
	}
	var b [3]strings.Builder
	for i, r := range runes {
		glyph, ok := blockFont[r]
		if !ok {
			return rows, false
		}
		for row := 0; row < 3; row++ {
			if i > 0 {
				b[row].WriteByte(' ')
			}
			for col := 0; col < 3; col++ {
				top := glyph[(row*2)*3+col] == '#'
				bottom := row*2+1 < 5 && glyph[(row*2+1)*3+col] == '#'
				switch {
				case top && bottom:
					b[row].WriteString("█")
				case top:
					b[row].WriteString("▀")
				case bottom:
					b[row].WriteString("▄")
				default:
					b[row].WriteByte(' ')
				}
			}
		}
	}
	for i := range rows {
		rows[i] = b[i].String()
	}
	return rows, true
}
