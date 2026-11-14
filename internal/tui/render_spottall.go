package tui

import (
	"fmt"
	"strings"

	"github.com/aklkbqx/wol/internal/store"
	"github.com/aklkbqx/wol/internal/termimg"
)

// spotTallStage is the portrait stage for phones: the picture across the
// top, the name in plain bold (block letters break up under the line
// spacing phone terminals add), the facts, and buttons three rows tall so a
// thumb can hit them.
func (m *WakeModel) spotTallStage(devices []store.Device, f spotFrame, st spotStyles) []string {
	s := st.stage
	width, height := f.stage, f.height
	lines := spotBlankStage(width, height, st)
	now := m.now()
	note := m.spotFreshness(now)
	if chips := m.spotFilterChips(); chips != "" {
		note = chips
	}
	lines[1] = m.spotTabs(width, fitText(note, width/2), m.stale, st)
	if len(devices) == 0 {
		lines[6] = newSpotRow(width).add(s.text, "No machine matches this filter.").String(s.fill)
		lines[8] = newSpotRow(width).add(s.muted, "Press / or esc.").String(s.fill)
		m.spotTallFooter(lines, devices, width, height, st)
		return lines
	}
	device := devices[min(m.selected, len(devices)-1)]
	state := m.deviceState(device)
	waking := m.waking && device.ID == m.actionTargetID
	name := m.privateField(device.Name)

	top := 3
	rows := 0
	if m.artOn() {
		rows = min(min(width/3, height-8-spotTallBlock-top-1), min(spotArtMaxRows, termimg.MaxPlaceholder))
	}
	var photo bool
	var picNote string
	if rows >= spotArtMinRows {
		var pic []string
		pic, photo, picNote = m.stagePicture(device, rows*3, rows, now)
		for i := 0; i < rows && i < len(pic); i++ {
			lines[top+i] = newSpotRow(width).raw(pic[i], rows*3).String(s.fill)
		}
		top += rows + 1
	} else {
		// No picture: keep the machine and its buttons near the thumb,
		// just above the place in the list, not under the tabs.
		top = max(top+1, height-8-spotTallBlock)
	}
	lay := spotLayout{nameRow: top, stateRow: top + 1, typeRow: top + 2, factsRow: top + 4, actionRow: top + 10, plain: true}
	rowAt := func(int) *spotRow { return newSpotRow(width) }

	dot := s.muted
	switch {
	case waking:
		dot = s.amber
	case state == "ONLINE":
		dot = s.green
	case state == "DISABLED":
		dot = s.rose
	}
	lines[lay.nameRow] = newSpotRow(width).add(dot, "●").add(s.fill, " ").add(s.bold, fitText(name, width-2)).String(s.fill)
	kind, _ := m.artKind(device)
	typeRow := newSpotRow(width).add(s.muted, kind.Label())
	if photo {
		typeRow.add(s.muted, "  ·  your picture")
	} else if picNote != "" {
		typeRow.add(s.muted, "  ·  "+picNote)
	}

	if waking {
		m.spotWaking(lines, device, width, now, st, lay, rowAt)
		m.spotTallFooter(lines, devices, width, height, st)
		return lines
	}
	lines[lay.typeRow] = typeRow.String(s.fill)
	m.spotDetails(lines, device, state, name, width, st, lay, rowAt)

	var btns [][2]string
	switch {
	case m.opening || m.shuttingDown || (!m.isWakeReady(device) && state != "ONLINE"):
	case state == "ONLINE":
		if _, ok := m.profiles[device.ID]; ok {
			btns = [][2]string{{"c", "Connect"}, {"s", "check"}}
		} else {
			btns = [][2]string{{"s", "Check"}, {"p", "remote"}}
		}
	default:
		btns = [][2]string{{"w", "Wake " + fitText(name, 14)}, {"c", "connect"}}
		if visibleWidth(btns[0][1])+29 > width {
			btns[0][1] = "Wake" // the name is just above
		}
	}
	if btns != nil {
		var r [3]*spotRow
		for i := range r {
			r[i] = newSpotRow(width)
		}
		for i, b := range btns {
			label := "  " + b[0] + "   " + b[1] + "  "
			w := visibleWidth(label)
			gap := 0
			if i > 0 {
				gap = 2
			}
			if r[1].used+gap+w > width {
				break
			}
			style := st.button
			if i > 0 {
				style = st.keycap
			}
			for j := range r {
				r[j].add(s.fill, spaces(gap))
				if j == 1 {
					r[j].add(style, label)
				} else {
					r[j].add(style, spaces(w))
				}
			}
		}
		for j := range r {
			lines[lay.actionRow-1+j] = r[j].String(s.fill)
		}
	}
	m.spotTallFooter(lines, devices, width, height, st)
	return lines
}

// spotTallBlock is the rows from the name to the blank row under the buttons.
const spotTallBlock = 13

// spotTallFooter writes the place in the list, the status wrapped to two
// rows instead of cut, and the keys.
func (m *WakeModel) spotTallFooter(lines []string, devices []store.Device, width, height int, st spotStyles) {
	s := st.stage
	busy := m.waking || m.opening || m.shuttingDown
	switch {
	case m.filtering:
		lines[height-8] = newSpotRow(width).add(s.muted, "filter ").add(s.amber, "/ ").add(s.text, fitText(m.privateField(m.filterInput), width-10)).add(s.amber, "_").String(s.fill)
	case len(devices) > 0 && !busy:
		i := min(m.selected, len(devices)-1)
		row := newSpotRow(width).add(s.muted, fmt.Sprintf("%d of %d", i+1, len(devices)))
		col := row.used + 3
		half := col + (width-col)/2
		if i > 0 {
			row.to(s.fill, col).add(s.muted, "↑ ").add(s.soft, fitText(m.privateField(devices[i-1].Name), half-col-3))
		}
		if i+1 < len(devices) {
			row.to(s.fill, half).add(s.muted, "↓ ").add(s.soft, fitText(m.privateField(devices[i+1].Name), width-half-2))
		}
		lines[height-8] = row.String(s.fill)
	}
	if msg := strings.TrimSpace(m.status); msg != "" && !busy {
		style := s.muted
		if statusNeedsAttention(msg) {
			style = s.rose
		}
		wrapped := spotWrap(m.privateMessage(msg), width)
		if len(wrapped) > 2 {
			wrapped[1] = fitText(wrapped[1]+" "+wrapped[2], width)
		}
		for i := 0; i < len(wrapped) && i < 2; i++ {
			lines[height-6+i] = newSpotRow(width).add(style, wrapped[i]).String(s.fill)
		}
	}
	// Hints that do not fit drop from the end, so keys and the list come
	// before filter on a small phone.
	hints := []spotHint{{key: "↑↓", label: "move"}, {key: "?", label: "keys"}, {key: "l", label: "list"}, {key: "/", label: "filter"}}
	switch {
	case busy:
		hints = []spotHint{{key: "esc", label: "cancel"}}
	case m.filtering:
		hints = []spotHint{{key: "enter", label: "apply"}, {key: "esc", label: "cancel"}}
	}
	lines[height-3] = m.spotKeyRow(width, st, hints, spotHint{key: "q", label: "quit"})
}
