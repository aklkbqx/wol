package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/aklkbqx/wol/internal/store"
)

// Small Spotlight frames. Below the narrow pane the stage stands alone and
// names the machines above and below the focused one; below that, or when
// l asks for it, the list stands alone with the focused machine's route on
// the row under it.

// spotSingleChrome writes what the stage alone needs in place of the
// pane: the filter on the tab row, the place in the list and the machines
// above and below the focused one, and keys that include the list.
func (m *WakeModel) spotSingleChrome(lines []string, devices []store.Device, f spotFrame, st spotStyles) {
	s := st.stage
	width, height := f.stage, f.height
	if chips := m.spotFilterChips(); chips != "" {
		lines[1] = m.spotTabs(width, fitText(chips, width/2), false, st)
	}
	if m.waking || m.opening || m.shuttingDown {
		return
	}
	switch {
	case m.filtering:
		row := newSpotRow(width).add(s.muted, "filter ").add(s.amber, "/ ").add(s.text, fitText(m.privateField(m.filterInput), width-10)).add(s.amber, "_")
		lines[height-5] = row.String(s.fill)
	case m.status == "" && len(devices) > 0:
		i := min(m.selected, len(devices)-1)
		row := newSpotRow(width).add(s.muted, fmt.Sprintf("%d of %d", i+1, len(devices)))
		col := row.used + 4
		half := col + (width-col)/2
		if i > 0 {
			row.to(s.fill, col).add(s.muted, "↑ ").add(s.soft, fitText(m.privateField(devices[i-1].Name), half-col-4))
		}
		if i+1 < len(devices) {
			row.to(s.fill, half).add(s.muted, "↓ ").add(s.soft, fitText(m.privateField(devices[i+1].Name), width-half-2))
		}
		lines[height-5] = row.String(s.fill)
	}
	lines[height-3] = m.spotKeyRow(width, st, []spotHint{
		{key: "↑↓", label: "move"}, {key: "l", label: "list"}, {key: "/", label: "filter"}, {key: "?", label: "keys"},
	}, spotHint{key: "q", label: "quit"})
}

// renderSpotList draws the list alone: one row per machine with its state
// in words, so the frame reads without colour.
func (m *WakeModel) renderSpotList(devices []store.Device, f spotFrame, st spotStyles) string {
	s := st.stage
	width, height := f.width, f.height
	// The list keeps to the stage measure on a wide, short window.
	const margin = 2
	inner := min(width-2*margin, spotMeasure)
	// A wide, short window (a phone on its side) puts the focused machine
	// beside the list instead of leaving the right half empty.
	listW, detW := inner, 0
	if width >= spotMinWidth && height-8 >= 8 {
		listW = spotSideWidth
		detW = min(width-2*margin-listW-4, 46)
		inner = listW + 4 + detW
	}
	right := width - margin - inner
	lines := spotBlankStage(width, height, st)
	pad := func(row *spotRow) string {
		return s.fill.pre + spaces(margin) + s.fill.post + row.String(s.fill) + s.fill.pre + spaces(right) + s.fill.post
	}

	head := newSpotRow(inner)
	if logo := m.paneLogo(); logo != "" {
		head.raw(logo, 2)
	} else {
		head.add(s.amber, "●")
	}
	head.add(s.fill, " ").add(s.bold, "wol")
	note := fmt.Sprintf("%d %s", len(devices), plural(len(devices), "machine", "machines"))
	if chips := m.spotFilterChips(); chips != "" {
		note = chips + "  ·  " + note
	}
	note = fitText(note, inner-head.used-3)
	head.to(s.fill, inner-visibleWidth(note)).add(s.muted, note)
	lines[1] = pad(head)

	// Rows 3..height-5 hold the list, one row per machine.
	first, last := 3, height-5
	slots := last - first + 1
	selected := min(m.selected, max(0, len(devices)-1))
	start := 0
	if selected >= slots {
		start = selected - slots + 1
	}
	detail := m.spotListDetail(devices, selected, detW, slots, st)
	for i := 0; i < slots; i++ {
		if start+i >= len(devices) {
			if detW > 0 {
				row := newSpotRow(inner).to(s.fill, listW+4)
				detail(i, row)
				lines[first+i] = pad(row)
			}
			continue
		}
		idx := start + i
		device := devices[idx]
		focused := idx == selected
		tone := s
		nameStyle := s.soft
		if focused {
			tone, nameStyle = st.focus, st.focus.bold
		}
		word := spotStateWord(m.deviceState(device))
		if m.waking && device.ID == m.actionTargetID {
			word = "waking"
		}
		glyph, gStyle := m.spotGlyph(device, tone)
		row := newSpotRow(inner).add(tone.fill, " ").add(gStyle, glyph).add(tone.fill, " ")
		tail := len(word) + 2
		if m.marked[device.ID] {
			tail += 3
		}
		row.add(nameStyle, fitText(m.privateField(device.Name), listW-row.used-tail-1))
		if m.marked[device.ID] {
			row.to(tone.fill, listW-tail).add(tone.amber, "✓")
		}
		row.to(tone.fill, listW-len(word)-1).add(tone.muted, word).to(tone.fill, listW)
		if detW > 0 {
			row.to(s.fill, listW+4)
			detail(i, row)
		}
		row.to(s.fill, inner)
		lines[first+i] = pad(row)
	}
	if len(devices) == 0 {
		empty := "Nothing matches. Press / or esc."
		if len(m.devices) == 0 {
			empty = "No machines yet. Press a to add one."
		}
		lines[first] = pad(newSpotRow(inner).add(s.muted, fitText(empty, inner)))
	} else if start+slots < len(devices) && slots > 1 {
		more := fmt.Sprintf("+%d more", len(devices)-start-slots)
		lines[last] = pad(newSpotRow(inner).add(s.muted, more))
	}

	// The row under the list: the filter being typed, the status, the
	// wake countdown or how the focused machine is checked and woken.
	info := newSpotRow(inner)
	switch {
	case m.filtering:
		info.add(s.muted, "filter ").add(s.amber, "/ ").add(s.text, fitText(m.privateField(m.filterInput), inner-10)).add(s.amber, "_")
	case m.waking && len(devices) > 0:
		left := wakeAnswerWindow
		if m.action == "wake-wait" && !m.wakeSentAt.IsZero() {
			left = wakeAnswerWindow - m.now().Sub(m.wakeSentAt)
		}
		if left < 0 {
			left = 0
		}
		secs := int(left.Round(time.Second).Seconds())
		info.add(s.amber, "waking ").add(s.soft, fitText(m.privateField(m.actionTarget), inner-16)).add(s.muted, fmt.Sprintf("  %d:%02d", secs/60, secs%60))
	case m.status != "":
		style := s.muted
		if statusNeedsAttention(m.status) {
			style = s.rose
		}
		info.add(style, fitText(m.privateMessage(m.status), inner))
	case detW > 0:
	case len(devices) > 0:
		device := devices[selected]
		checks := "port sweep"
		if port := m.verifyPort(device); port > 0 {
			checks = fmt.Sprintf("tcp:%d", port)
		}
		info.add(s.muted, fitText(checks+" · "+m.privateMessage(m.routeText(device)), inner))
	}
	lines[height-4] = pad(info)

	hints := []spotHint{{key: "w", label: "wake", primary: true}, {key: "?", label: "keys"}}
	if m.waking || m.opening || m.shuttingDown {
		hints = []spotHint{{key: "esc", label: "cancel"}}
	} else if len(m.devices) == 0 {
		hints = []spotHint{{key: "a", label: "add", primary: true}, {key: "n", label: "scan"}}
	} else if m.listView && width >= spotSingleWidth && height >= spotStageHeight {
		hints = []spotHint{{key: "w", label: "wake", primary: true}, {key: "l", label: "back"}, {key: "?", label: "keys"}}
	}
	if detW > 0 && len(hints) > 0 && hints[0].key == "w" {
		hints[0].primary = false // the Wake button beside the list is the one amber action
	}
	lines[height-2] = s.fill.pre + spaces(margin) + s.fill.post + m.spotKeyRow(inner, st, hints, spotHint{key: "q", label: "quit"}) + s.fill.pre + spaces(right) + s.fill.post
	return strings.Join(lines, "\n")
}

// spotListDetail returns a writer for the column beside a landscape list:
// the focused machine's name, state, facts and main action.
func (m *WakeModel) spotListDetail(devices []store.Device, selected, width, rows int, st spotStyles) func(int, *spotRow) {
	if width <= 0 || len(devices) == 0 {
		return func(int, *spotRow) {}
	}
	s := st.stage
	device := devices[selected]
	state := m.deviceState(device)
	name := m.privateField(device.Name)
	waking := m.waking && device.ID == m.actionTargetID
	addr := strings.TrimSpace(device.IPAddress)
	if addr == "" {
		addr = "no address"
	}
	checks := "port sweep and ping"
	if port := m.verifyPort(device); port > 0 {
		checks = fmt.Sprintf("tcp:%d", port)
	}
	word, wordStyle := spotStateWord(state), s.soft
	if state == "ONLINE" {
		wordStyle = s.green
	}
	if waking {
		word, wordStyle = "waking", s.amber
	}
	facts := [][2]string{{"address", m.privateField(addr)}, {"hardware", m.privateField(device.MACAddress)}, {"wakes by", m.privateMessage(m.routeText(device))}, {"checks", checks}}
	return func(i int, row *spotRow) {
		base := row.used
		switch {
		case i == 0:
			row.add(s.bold, fitText(name, width))
		case i == 1:
			row.add(wordStyle, word).add(s.fill, "   ").add(s.muted, fitText(m.spotStateDetail(device, state), width-len(word)-3))
		case i >= 3 && i < 7:
			f := facts[i-3]
			row.add(s.muted, f[0]).to(s.fill, base+10).add(s.text, fitText(f[1], width-10))
		case i == rows-1 && !waking && state != "ONLINE" && m.isWakeReady(device):
			row.add(st.button, "  w   Wake "+fitText(name, width-14)+"  ")
		}
	}
}
