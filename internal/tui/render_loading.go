package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m *WakeModel) renderRemoteLoadingView(width int, mode LayoutMode) string {
	var builder strings.Builder
	builder.WriteString("\n" + fitText(m.theme.title().Render("wol"), width) + "\n\n")
	target := m.actionTarget
	if strings.TrimSpace(target) == "" {
		target = "machine"
	}
	last := "REMOTE"
	if profile, ok := m.profiles[m.actionTargetID]; ok && streamProfile(profile) {
		last = "STREAM"
	}
	builder.WriteString(fitText(m.theme.accent().Render(target), width) + "\n\n")
	if mode == LayoutNarrow || width < 28 {
		connector := "v"
		builder.WriteString(m.theme.accent().Render("WAKE\n"+connector+"\nWAIT\n"+connector+"\n"+last) + "\n\n")
	} else {
		builder.WriteString(m.renderActionPath([]string{"WAKE", "WAIT", last}, width) + "\n\n")
	}
	builder.WriteString(fitText(m.theme.muted().Render("esc cancel   q quit"), width) + "\n")
	return builder.String()
}

func (m *WakeModel) renderLoadingView(width int, mode LayoutMode) string {
	var builder strings.Builder
	builder.WriteString("\n" + fitText(m.theme.title().Render("wol"), width) + "\n")
	if m.phase == phaseLoadError {
		builder.WriteString("\n" + fitText(m.theme.danger().Render("could not load"), width) + "\n\n")
		message := m.loadingError
		if message == "" {
			message = "The local inventory could not be read."
		}
		builder.WriteString(fitText(message, width) + "\n\n")
		builder.WriteString(fitText(m.theme.accent().Render("r retry")+m.theme.muted().Render("   q quit"), width) + "\n")
		return builder.String()
	}

	title := "checking"
	if m.phase == phaseCheckingMachine {
		title = "checking power"
	} else if m.phase == phaseRefreshing {
		title = "refreshing"
	}
	builder.WriteString("\n" + fitText(m.theme.muted().Render(title), width) + "\n")
	if m.loadingTarget != "" {
		builder.WriteString(fitText(m.theme.accent().Render(m.loadingTarget), width) + "\n")
	}
	builder.WriteString("\n" + fitText(m.loadingSignal(width), width) + "\n\n")

	count := 0
	if m.pending != nil {
		count = len(m.pending.devices)
	}
	stage := "reading inventory"
	if m.loadingStage == stagePresence || m.phase == phaseCheckingMachine {
		stage = "checking power"
		if count > 0 {
			stage = fmt.Sprintf("checking %d machines", count)
		}
	}
	builder.WriteString(fitText(m.theme.accent().Render(stage), width) + "\n\n")
	controls := "q quit"
	if m.phase == phaseRefreshing || m.phase == phaseCheckingMachine {
		controls = "esc cancel   q quit"
	}
	builder.WriteString(fitText(m.theme.muted().Render(controls), width) + "\n")
	return builder.String()
}

func (m *WakeModel) loadingSignal(width int) string {
	left, right := "LOCAL", "FLEET"
	if m.phase == phaseCheckingMachine {
		right = "TARGET"
	}
	railWidth := width - len(left) - len(right) - 2
	if railWidth < 1 {
		return m.theme.accent().Render(m.theme.Glyph("signal-busy") + " CHECKING")
	}
	railWidth = min(railWidth, 30)
	railRune, signalRune := m.theme.Glyph("rail"), m.theme.Glyph("signal-ready")
	position := railPosition(time.Now(), m.motion, railWidth)
	rail := strings.Repeat(railRune, position) + signalRune + strings.Repeat(railRune, railWidth-position-1)
	return m.theme.muted().Render(left+" ") + m.theme.accent().Render(rail) + m.theme.muted().Render(" "+right)
}

func railPosition(now time.Time, motion Motion, railWidth int) int {
	if railWidth <= 1 || !motion.Enabled || motion.Started.IsZero() {
		return railWidth / 2
	}
	const period = 1200 * time.Millisecond
	elapsed := now.Sub(motion.Started)
	if elapsed < 0 {
		elapsed = 0
	}
	cycle := elapsed % (2 * period)
	var t float64
	if cycle < period {
		t = float64(cycle) / float64(period)
	} else {
		t = 1 - float64(cycle-period)/float64(period)
	}
	return int(t * float64(railWidth-1))
}
