package tui

import (
	"strings"
	"testing"

	"github.com/aklkbqx/wol/internal/fleet"
)

// spotPanelCases opens each Spotlight panel on the spotlight model.
var spotPanelCases = []struct {
	kind  string
	setup func(*WakeModel)
	want  []string
}{
	{panelHelp, func(m *WakeModel) { m.showHelp = true }, []string{"All keys", "This machine", "Fleet and view", "next palette", "Night Room", "v0.4.20-dev"}},
	{panelPicker, func(m *WakeModel) { m.actionPicker = true; m.pickerSelected = 1 }, []string{"nas-vault", "Wake", "Stream", "Check", "Power off", "Cancel", "choose"}},
	{panelConfirm, func(m *WakeModel) { m.confirm = "machine:b" }, []string{"Delete nas-vault?", "sent to the machine.", "02:00:00:00:01:02", "keep it"}},
	{panelPreview, func(m *WakeModel) { m.batchKind = "wake"; m.batchTargets = m.devices }, []string{"Wake 5 machines", "5 selected", "print-srv", "wake 5"}},
	{panelResults, func(m *WakeModel) {
		m.showBatchResults = true
		m.lastBatchKind = "wake"
		m.batchTargets = m.devices
		m.batchResults = []fleet.Result{{Name: "nas-vault", Status: "sent"}, {Name: "studio-ws", Status: "failed", Message: "no route"}, {Name: "lab-mini", Status: "cancelled"}}
	}, []string{"Wake results", "3 of 5", "no route", "1 sent", "1 failed", "1 cancelled"}},
	{panelEmpty, func(m *WakeModel) { m.devices = nil; m.marked = nil }, []string{"No machines yet", "no machines yet", "Add a machine", "scan network"}},
	{panelOpening, func(m *WakeModel) {
		m.opening = true
		m.actionTargetID, m.actionTarget = "b", "nas-vault"
	}, []string{"Connecting to nas-vault", "Wake", "Wait", "Stream", "cancel"}},
}

func TestSpotPanelsDrawExactFrames(t *testing.T) {
	for _, tc := range spotPanelCases {
		for _, size := range [][2]int{{90, 24}, {100, 28}, {140, 40}} {
			for _, color := range []bool{true, false} {
				m := spotlightModel()
				m.width, m.height = size[0], size[1]
				m.theme = NewTheme(color, false)
				tc.setup(m)
				if got := m.spotPanelKind(m.width, m.height); got != tc.kind {
					t.Fatalf("%s %dx%d: panel kind %q", tc.kind, size[0], size[1], got)
				}
				view := m.render()
				assertExactFrame(t, view, size[0], size[1])
				if size[0] < 100 {
					continue
				}
				plain := stripANSI(view)
				for _, want := range tc.want {
					if !strings.Contains(plain, want) {
						t.Errorf("%s %dx%d color=%t: missing %q:\n%s", tc.kind, size[0], size[1], color, want, plain)
					}
				}
			}
		}
	}
}

func TestSpotPanelsFallBackToClassicLayout(t *testing.T) {
	cases := map[string]func(*WakeModel){
		"list help":     func(m *WakeModel) { m.showHelp = true; m.width = 49 },
		"short picker":  func(m *WakeModel) { m.actionPicker = true; m.height = 19 },
		"ascii confirm": func(m *WakeModel) { m.confirm = "machine:b"; m.theme = NewTheme(false, true) },
		"route confirm": func(m *WakeModel) { m.tab = 1; m.confirm = "relay:r" },
		"site confirm":  func(m *WakeModel) { m.confirm = "site:s" },
		"routes help":   func(m *WakeModel) { m.tab = 1; m.showHelp = true },
		"lan open":      func(m *WakeModel) { m.lanOpen = true; m.showHelp = true },
		"form": func(m *WakeModel) {
			m.form = newWakeForm(deviceForm, "b", []string{"Name"}, []string{"nas-vault"}, m.theme)
		},
		"machines":      func(m *WakeModel) {},
		"running batch": func(m *WakeModel) { m.batchKind, m.batchRunning = "wake", true; m.batchTargets = m.devices },
	}
	for name, setup := range cases {
		m := spotlightModel()
		setup(m)
		if kind := m.spotPanelKind(m.width, m.height); kind != "" {
			t.Errorf("%s: got panel %q, want none", name, kind)
		}
	}
}

// Keys act on the panel that is drawn: a batch preview opened from the
// results view wins, as it does in handleKey.
func TestSpotPanelOrderFollowsKeys(t *testing.T) {
	m := spotlightModel()
	m.showBatchResults = true
	m.batchKind, m.batchTargets = "check", m.devices
	if got := m.spotPanelKind(m.width, m.height); got != panelPreview {
		t.Fatalf("panel = %q, want preview", got)
	}
	m.batchKind = ""
	m.actionPicker, m.showHelp = true, true
	m.showBatchResults = false
	if got := m.spotPanelKind(m.width, m.height); got != panelPicker {
		t.Fatalf("panel = %q, want picker", got)
	}
}

func TestSpotPanelsRedactIdentifiers(t *testing.T) {
	for _, tc := range spotPanelCases {
		m := spotlightModel()
		tc.setup(m)
		m.redactIdentifiers = true
		m.devices = append(m.devices[:0:0], m.devices...)
		plain := stripANSI(m.render())
		for _, secret := range []string{"nas-vault", "studio-ws", "198.51.100", "02:00:00", "no route"} {
			if tc.kind == panelResults && secret == "no route" {
				continue // a plain message with no identifier stays readable
			}
			if strings.Contains(plain, secret) {
				t.Errorf("%s leaked %q:\n%s", tc.kind, secret, plain)
			}
		}
	}
}

func TestSpotPreviewScrollsTargets(t *testing.T) {
	m := spotlightModel()
	m.height = 24
	m.batchKind = "check"
	for i := 0; i < 4; i++ {
		m.batchTargets = append(m.batchTargets, m.devices...)
	}
	m.batchPreviewIndex = len(m.batchTargets) - 1
	view := m.render()
	assertExactFrame(t, view, 100, 24)
	plain := stripANSI(view)
	if !strings.Contains(plain, "above") || !strings.Contains(plain, "scroll") {
		t.Fatalf("scrolled preview lost its markers:\n%s", plain)
	}
}

func TestSpotWrap(t *testing.T) {
	got := spotWrap("one two three four", 9)
	want := []string{"one two", "three", "four"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("spotWrap = %q, want %q", got, want)
	}
	if got := spotWrap("averyveryverylongword", 6); len(got) != 1 || visibleWidth(got[0]) > 6 {
		t.Fatalf("long word not cut to width: %q", got)
	}
}

func BenchmarkRenderSpotHelp(b *testing.B) {
	m := spotlightModel()
	m.showHelp = true
	b.ReportAllocs()
	for b.Loop() {
		_ = m.render()
	}
}

func BenchmarkRenderSpotPreview(b *testing.B) {
	m := spotlightModel()
	m.batchKind, m.batchTargets = "wake", m.devices
	b.ReportAllocs()
	for b.Loop() {
		_ = m.render()
	}
}
