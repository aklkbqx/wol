package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/aklkbqx/wol/internal/termimg"
)

func TestSpotFrameModes(t *testing.T) {
	cases := []struct {
		w, h        int
		mode        int
		pane, stage int
		tall        bool
	}{
		{160, 44, spotFull, 32, 123, false},
		{90, 24, spotFull, 32, 53, false},
		{90, 20, spotFull, 32, 53, false},
		{89, 24, spotNarrow, 24, 61, false},
		{72, 20, spotNarrow, 24, 44, false},
		{71, 30, spotSingle, 0, 65, false},
		{50, 20, spotSingle, 0, 44, false},
		{49, 40, spotSingle, 0, 45, true},
		{52, 38, spotSingle, 0, 46, true},
		{71, 36, spotSingle, 0, 65, true},
		{40, 30, spotSingle, 0, 36, true},
		{35, 40, spotList, 0, 0, false},
		{52, 29, spotSingle, 0, 46, false},
		{72, 40, spotNarrow, 24, 44, false},
		{120, 19, spotList, 0, 0, false},
		{30, 10, spotList, 0, 0, false},
		{29, 24, spotNone, 0, 0, false},
		{80, 9, spotNone, 0, 0, false},
	}
	for _, tc := range cases {
		m := spotlightModel()
		f := m.spotFrame(tc.w, tc.h)
		if f.mode != tc.mode || f.pane != tc.pane || f.stage != tc.stage || f.tall != tc.tall {
			t.Errorf("%dx%d: mode %d pane %d stage %d tall %t, want %d %d %d %t", tc.w, tc.h, f.mode, f.pane, f.stage, f.tall, tc.mode, tc.pane, tc.stage, tc.tall)
		}
		if f.hasStage() && f.pane+f.gutter+f.stage+f.right != tc.w {
			t.Errorf("%dx%d: columns do not add up: %+v", tc.w, tc.h, f)
		}
	}
	m := spotlightModel()
	m.listView = true
	if f := m.spotFrame(60, 20); f.mode != spotList {
		t.Fatalf("l on a single-column window: mode %d, want list", f.mode)
	}
	if f := m.spotFrame(52, 38); f.mode != spotList {
		t.Fatalf("l on a portrait window: mode %d, want list", f.mode)
	}
	if f := m.spotFrame(80, 24); f.mode != spotNarrow {
		t.Fatalf("l with room for the pane: mode %d, want narrow", f.mode)
	}
}

// Every size from the smallest list to a wide window draws a full frame,
// for the machine view and every panel and form that uses the stage.
func TestSpotFramesFillEverySize(t *testing.T) {
	setups := map[string]func(*WakeModel){"machines": func(*WakeModel) {}}
	for _, tc := range spotPanelCases {
		setups[tc.kind] = tc.setup
	}
	setups["form"] = func(m *WakeModel) { m.beginEdit() }
	setups["waking"] = func(m *WakeModel) {
		m.waking, m.action, m.actionTargetID, m.actionTarget = true, "wake", "b", "nas-vault"
	}
	setups["filtering"] = func(m *WakeModel) { m.filtering, m.filterInput = true, "nas" }
	setups["list"] = func(m *WakeModel) { m.listView = true }
	setups["picture"] = func(m *WakeModel) { m.art = newArtCache(termimg.Blocks) }
	setups["status"] = func(m *WakeModel) { m.status = spotLongStatus }
	for name, setup := range setups {
		for _, h := range []int{10, 14, 19, 20, 23, 24, 30, 38, 52} {
			for w := spotListWidth; w <= 130; w++ {
				for _, color := range []bool{true, false} {
					m := spotlightModel()
					m.width, m.height = w, h
					m.theme = NewTheme(color, false)
					setup(m)
					view, ok := m.spotView(w, h)
					if !ok {
						continue
					}
					lines := strings.Split(view, "\n")
					if len(lines) != h {
						t.Fatalf("%s %dx%d: %d lines", name, w, h, len(lines))
					}
					for i, line := range lines {
						if got := visibleWidth(line); got != w {
							t.Fatalf("%s %dx%d color=%t: line %d is %d wide:\n%s", name, w, h, color, i, got, stripANSI(view))
						}
					}
				}
			}
		}
	}
}

func TestSpotSingleNamesNeighbours(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 60, 20
	plain := stripANSI(m.render())
	for _, want := range []string{"Machines", "2 of 5", "↑ studio-ws", "↓ render-node", "address", "198.51.100.20", "Wake nas-vault", "list", "quit"} {
		if !strings.Contains(plain, want) {
			t.Errorf("single frame missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "MACHINES") {
		t.Fatalf("single frame drew the pane:\n%s", plain)
	}
}

func TestSpotNarrowPaneStacksFacts(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 80, 24
	plain := stripANSI(m.render())
	for _, want := range []string{"MACHINES", "studio-ws", "address   198.51.100.20", "hardware  02:00:00:00:01:02", "checks    tcp:445", "Wake nas-vault"} {
		if !strings.Contains(plain, want) {
			t.Errorf("narrow frame missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "1 m ") {
		t.Fatalf("pane summary cut mid-word:\n%s", plain)
	}
}

func TestSpotListReadsWithoutColour(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 44, 14
	m.theme = NewTheme(false, false)
	view := m.render()
	assertExactFrame(t, view, 44, 14)
	if strings.Contains(view, "38;2;") {
		t.Fatalf("no-color list emitted colour codes")
	}
	plain := stripANSI(view)
	for _, want := range []string{"5 machines", "studio-ws", "awake", "asleep", "unknown", "✓", "tcp:445", "wake", "quit"} {
		if !strings.Contains(plain, want) {
			t.Errorf("list missing %q:\n%s", want, plain)
		}
	}
}

func TestSpotListKeyTogglesAndEscCloses(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 60, 20
	m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	if !m.listView || !strings.Contains(stripANSI(m.render()), "back") {
		t.Fatalf("l did not open the list")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.listView {
		t.Fatalf("esc did not close the list")
	}
	if len(m.marked) == 0 {
		t.Fatalf("esc closing the list also cleared the marks")
	}
}

func TestSpotHelpPagesShowEveryKey(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 72, 20
	m.showHelp = true
	var seen strings.Builder
	first := stripANSI(m.render())
	if !strings.Contains(first, "more keys 1/") {
		t.Fatalf("short help has no pages:\n%s", first)
	}
	for i := 0; i < 4; i++ {
		seen.WriteString(stripANSI(m.render()))
		m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	}
	for _, col := range spotHelpKeys {
		for _, k := range col {
			if !strings.Contains(seen.String(), k[1]) {
				t.Errorf("no page shows %q", k[1])
			}
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.showHelp || m.helpPage != 0 {
		t.Fatalf("esc left help open or kept the page")
	}
}

func TestSpotSmallFramesRedact(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 20}, {44, 14}, {52, 38}, {104, 16}} {
		m := spotlightModel()
		m.width, m.height = size[0], size[1]
		m.redactIdentifiers = true
		m.status = "Woke nas-vault at 198.51.100.20."
		plain := stripANSI(m.render())
		for _, secret := range []string{"nas-vault", "studio-ws", "render-node", "198.51.100", "02:00:00"} {
			if strings.Contains(plain, secret) {
				t.Errorf("%dx%d leaked %q:\n%s", size[0], size[1], secret, plain)
			}
		}
	}
}

const spotLongStatus = "Checked 5/5 machines. Unreachable does not prove power is off."

// A phone held upright gets the portrait stage: the name in plain bold, the
// facts, tall buttons, the neighbours and a status that wraps instead of
// being cut.
func TestSpotTallStage(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 52, 38
	m.status = spotLongStatus
	view := m.render()
	assertExactFrame(t, view, 52, 38)
	lines := strings.Split(stripANSI(view), "\n")
	plain := strings.Join(lines, "\n")
	for _, want := range []string{"● nas-vault", "address   198.51.100.20", "checks    tcp:445", "w   Wake nas-vault", "c   connect", "2 of 5", "↑ studio-ws", "↓ render-node", "Unreachable does not", "prove power is off.", "?  keys", "l  list"} {
		if !strings.Contains(plain, want) {
			t.Errorf("tall stage missing %q:\n%s", want, plain)
		}
	}
	if strings.ContainsAny(plain, "█▀") {
		t.Fatalf("tall stage drew block letters:\n%s", plain)
	}
	// The button is three rows of one surface: the rows above and below the
	// label carry the same background.
	button := spotRowOf(lines, "Wake nas-vault")
	raw := strings.Split(view, "\n")
	for _, r := range []int{button - 1, button + 1} {
		if strings.TrimSpace(lines[r]) != "" || raw[r] == raw[button+2] {
			t.Errorf("button row %d is not filled:\n%q", r, raw[r])
		}
	}
	if strip := spotRowOf(lines, "2 of 5"); strip <= button+1 {
		t.Fatalf("place in the list (row %d) overlaps the button (row %d)", strip, button)
	}
}

func TestSpotTallStageWithPicture(t *testing.T) {
	for _, size := range [][2]int{{52, 38}, {40, 44}, {71, 36}} {
		m := spotlightModel()
		m.art = newArtCache(termimg.Blocks)
		m.width, m.height = size[0], size[1]
		view := m.render()
		assertExactFrame(t, view, size[0], size[1])
		lines := strings.Split(stripANSI(view), "\n")
		name, button, strip := spotRowOf(lines, "● nas-vault"), spotRowOf(lines, "   Wake"), spotRowOf(lines, "2 of 5")
		if name < 8 || !(name < button && button+1 < strip) {
			t.Fatalf("%dx%d: picture, name %d, button %d, strip %d out of order:\n%s", size[0], size[1], name, button, strip, strings.Join(lines, "\n"))
		}
	}
}

func TestSpotTallSmallPhone(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 40, 30
	plain := stripANSI(m.render())
	for _, want := range []string{"Machines  Routes  Activity  Sites", "w   Wake      c   connect", "?  keys", "q  quit"} {
		if !strings.Contains(plain, want) {
			t.Errorf("small phone missing %q:\n%s", want, plain)
		}
	}
}

func TestSpotTallWakingCountsDownInPlainText(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 52, 38
	m.waking, m.action, m.actionTargetID, m.actionTarget = true, "wake-wait", "b", "nas-vault"
	m.wakeSentAt = time.Now().Add(-12 * time.Second)
	plain := stripANSI(m.render())
	for _, want := range []string{"waking   packet sent 12 s ago", "1:18   seconds left of 90", "waiting for tcp:445", "esc  cancel"} {
		if !strings.Contains(plain, want) {
			t.Errorf("tall waking missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "█") || strings.Contains(plain, "2 of 5") {
		t.Fatalf("tall waking drew block digits or the place in the list:\n%s", plain)
	}
}

func TestSpotTallFilterKeys(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 52, 38
	m.filtering, m.filterInput = true, "nas"
	plain := stripANSI(m.render())
	for _, want := range []string{"filter / nas_", "enter  apply", "esc  cancel"} {
		if !strings.Contains(plain, want) {
			t.Errorf("tall filter missing %q:\n%s", want, plain)
		}
	}
}

// A wide, short window (a phone on its side) shows the focused machine
// beside the list.
func TestSpotWideListShowsDetail(t *testing.T) {
	m := spotlightModel()
	m.width, m.height = 104, 16
	view := m.render()
	assertExactFrame(t, view, 104, 16)
	plain := stripANSI(view)
	for _, want := range []string{"print-srv", "asleep   no answer", "address   198.51.100.20", "hardware  02:00:00:00:01:02", "checks    tcp:445", "w   Wake nas-vault"} {
		if !strings.Contains(plain, want) {
			t.Errorf("wide list missing %q:\n%s", want, plain)
		}
	}
	m.width = 89
	if plain := stripANSI(m.render()); strings.Contains(plain, "address   198.51.100.20") {
		t.Fatalf("89 columns drew the detail column:\n%s", plain)
	}
}

func spotRowOf(lines []string, text string) int {
	for i, l := range lines {
		if strings.Contains(l, text) {
			return i
		}
	}
	return -1
}

func BenchmarkRenderSpotNarrow(b *testing.B) { benchSpotSize(b, 80, 24) }
func BenchmarkRenderSpotSingle(b *testing.B) { benchSpotSize(b, 60, 20) }
func BenchmarkRenderSpotList(b *testing.B)   { benchSpotSize(b, 44, 14) }
func BenchmarkRenderSpotTall(b *testing.B)   { benchSpotSize(b, 52, 38) }
func BenchmarkRenderSpotWide(b *testing.B)   { benchSpotSize(b, 104, 16) }

func benchSpotSize(b *testing.B, w, h int) {
	m := spotlightModel()
	m.width, m.height = w, h
	b.ReportAllocs()
	for b.Loop() {
		_ = m.render()
	}
}
