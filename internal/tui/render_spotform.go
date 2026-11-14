package tui

import (
	"fmt"
	"strings"
)

// The Spotlight form keeps the machine pane on the left and puts the open
// form on the stage: one row per field, the focused field lit like the
// focused machine, and a plain-language hint for that field.

// spotFormEligible reports whether the open form can use the Spotlight
// frame. Forms opened from the other tabs keep the classic layout of
// their tab.
func (m *WakeModel) spotFormEligible(width, height int) bool {
	return m.form != nil && m.spotFrame(width, height).hasStage() && !m.showHelp && m.confirm == "" && !m.showBatchResults
}

// renderSpotForm draws the full frame, width × height cells.
func (m *WakeModel) renderSpotForm(width, height int) string {
	st := m.theme.spot()
	devices := m.filteredDevices()
	f := m.spotFrame(width, height)
	pane := m.spotPane(devices, f, st)
	stage := m.spotFormStage(f.stage, height, st)
	return spotCompose(pane, stage, f, st)
}

// formField is one form label split into what the stage shows.
type formField struct {
	label string // short label, no key hint
	hint  string // one line on what the field takes
	cycle bool   // ctrl+n cycles known values
}

// spotFormField describes field i of the open form.
func (m *WakeModel) spotFormField(i int) formField {
	form := m.form
	label := form.labels[i]
	f := formField{label: label}
	if open := strings.Index(label, " ("); open > 0 && strings.HasSuffix(label, ")") {
		f.label = label[:open]
		inner := label[open+2 : len(label)-1]
		if strings.EqualFold(inner, "Ctrl+N") {
			f.cycle = true
		} else {
			f.hint = inner
		}
	}
	if form.kind == deviceForm {
		switch i {
		case 0:
			f.hint = "required, the name you wake it by"
		case 1:
			f.hint = "required, six hex pairs from the network card"
		case 2:
			f.hint = "IPv4 used to check the machine, or blank"
		case 3:
			f.hint = "IPv4 to send the packet to, blank uses the site or 255.255.255.255"
		case 4:
			f.hint = "0 uses the site port, or 9"
		case 5:
			f.hint = "network interface to send from, blank lets the system pick"
		case 6:
			f.hint = "a port that answers once it is awake, 0 tries common ports"
		case 7:
			f.hint = "broadcast, or relay to wake through a route"
		case 8:
			f.hint = "ctrl+n picks one of your routes, blank sends directly"
		case deviceFieldSite:
			f.hint = "ctrl+n picks one of your sites"
		case deviceFieldType:
			f.hint = "ctrl+n cycles the types, blank guesses from the name"
		case deviceFieldPicture:
			f.hint = "a PNG, JPEG or GIF path, blank draws the room"
		}
	}
	if f.hint == "" && f.cycle {
		f.hint = "ctrl+n cycles the choices"
	}
	return f
}

// spotFormTitle names what the form edits.
func (m *WakeModel) spotFormTitle() string {
	form := m.form
	name := ""
	for _, d := range m.devices {
		if d.ID == form.id {
			name = m.privateField(d.Name)
			break
		}
	}
	switch form.kind {
	case deviceForm:
		if form.id == "" {
			return "New machine"
		}
		return "Edit " + name
	case remoteProfileForm:
		return "Remote for " + name
	case powerForm:
		return "Shut down " + name
	case importForm:
		return "Import inventory"
	case exportForm:
		return "Export inventory"
	case relayForm:
		return "Route"
	case siteForm:
		return "Site"
	}
	return "Edit"
}

func (m *WakeModel) spotFormStage(width, height int, st spotStyles) []string {
	s := st.stage
	form := m.form
	form.ensureInputs(m.theme)
	lines := make([]string, height)
	blank := s.fill.pre + spaces(width) + s.fill.post
	for i := range lines {
		lines[i] = blank
	}
	n := len(form.labels)
	note := fmt.Sprintf("field %d of %d", form.selected+1, n)
	if form.saving {
		note = "saving"
	}
	lines[1] = m.spotTabs(width, note, false, st)

	measure := min(width, spotMeasure)
	lines[4] = newSpotRow(width).add(s.bold, fitText(m.spotFormTitle(), measure)).String(s.fill)

	fields := make([]formField, n)
	labelW := 0
	for i := range fields {
		fields[i] = m.spotFormField(i)
		labelW = max(labelW, visibleWidth(fields[i].label))
	}
	labelW = min(labelW, 20)
	valueCol := 2 + labelW + 3

	// Rows top..bottom hold the fields. With room, every field gets a
	// spacer row below it and the focused field's hint sits there.
	// The first field lines up with the first machine in the pane.
	top, bottom := 6, height-6
	avail := bottom - top + 1
	step := 1
	if n*2-1 <= avail {
		step = 2
	}
	if n > avail {
		// Scrolling: keep the row above the status line for "↓ more".
		bottom--
		avail--
	}
	visible := min(n, avail)
	start := 0
	if n > visible {
		start = min(max(0, form.selected-visible/2), n-visible)
	}
	if start > 0 {
		lines[top-1] = newSpotRow(width).to(s.fill, 2).add(s.muted, fmt.Sprintf("↑ %d more", start)).String(s.fill)
	}
	if start+visible < n {
		lines[bottom+1] = newSpotRow(width).to(s.fill, 2).add(s.muted, fmt.Sprintf("↓ %d more", n-start-visible)).String(s.fill)
	}

	for k := 0; k < visible && top+k*step <= bottom; k++ {
		i := start + k
		focused := i == form.selected
		tone := s
		labelStyle := s.muted
		if focused {
			tone = st.focus
			labelStyle = tone.bold
		}
		row := newSpotRow(width)
		row.add(tone.fill, "  ").add(labelStyle, fitText(fields[i].label, labelW)).to(tone.fill, valueCol)
		room := measure - valueCol - 1
		if focused {
			m.spotFormInput(row, i, room, st)
			row.to(tone.fill, measure)
		} else {
			value := ""
			if i < len(form.values) {
				value = m.privateField(form.values[i])
			}
			if strings.TrimSpace(value) == "" {
				row.add(s.muted, "blank")
			} else {
				row.add(s.text, fitText(value, room))
			}
		}
		lines[top+k*step] = row.String(s.fill)
		if focused && step == 2 && fields[i].hint != "" && top+k*step+1 <= bottom {
			hint := newSpotRow(width).to(s.fill, valueCol).add(s.muted, fitText(fields[i].hint, width-valueCol))
			lines[top+k*step+1] = hint.String(s.fill)
		}
	}

	// The status line carries the error, or the hint when it had no room
	// beside the field.
	switch {
	case form.error != "":
		lines[height-5] = newSpotRow(width).add(s.rose, "✕ ").add(s.rose, fitText(m.privateMessage(form.error), width-2)).String(s.fill)
	case step == 1 && form.selected < n && fields[form.selected].hint != "":
		h := fields[form.selected]
		lines[height-5] = newSpotRow(width).add(s.soft, h.label).add(s.fill, "   ").add(s.muted, fitText(h.hint, width-visibleWidth(h.label)-3)).String(s.fill)
	}

	// Most needed first: narrow stages drop hints from the end.
	hints := []spotHint{{key: "ctrl+s", label: "save"}}
	if form.selected < n && fields[form.selected].cycle {
		hints = append(hints, spotHint{key: "ctrl+n", label: "cycle"})
	}
	hints = append(hints, spotHint{key: "tab", label: "next"})
	lines[height-3] = m.spotKeyRow(width, st, hints, spotHint{key: "esc", label: "cancel"})
	return lines
}

// spotFormInput writes the focused field's value with a block cursor,
// scrolled so the cursor stays in view.
func (m *WakeModel) spotFormInput(row *spotRow, i, room int, st spotStyles) {
	form := m.form
	tone := st.focus
	if room < 2 {
		return
	}
	if m.redactIdentifiers {
		value := ""
		if i < len(form.values) {
			value = m.privateField(form.values[i])
		}
		row.add(tone.text, fitText(value, room-1)).add(st.button, " ")
		return
	}
	runes := []rune(form.inputs[i].Value())
	pos := min(max(0, form.inputs[i].Position()), len(runes))
	start := 0
	if pos >= room {
		start = pos - room + 1
	}
	row.add(tone.text, string(runes[start:pos]))
	under := " "
	if pos < len(runes) {
		under = string(runes[pos])
	}
	row.add(st.button, under)
	if pos+1 < len(runes) {
		row.add(tone.text, fitText(string(runes[pos+1:]), room-(pos-start)-1))
	}
}
