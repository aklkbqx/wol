package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m *WakeModel) renderRemoteLoadingView(width int, mode LayoutMode) string {
	target := m.actionTarget
	if strings.TrimSpace(target) == "" {
		target = "machine"
	}
	last := "REMOTE"
	if profile, ok := m.profiles[m.actionTargetID]; ok && streamProfile(profile) {
		last = "STREAM"
	}

	// Compact mode for narrow/short viewports
	if width < 48 || m.height < 18 {
		var builder strings.Builder
		builder.WriteString("\n" + fitText(m.theme.title().Render("wol"), width) + "\n\n")
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

	// Modern Horizontally-Centered Cyber Remote Launcher
	cardWidth := min(width-4, 62)
	innerWidth := cardWidth - 4
	var body strings.Builder
	body.WriteString("\n")
	body.WriteString(m.theme.muted().Render("TARGET   : ") + m.theme.cyan().Render(fitText(target, innerWidth-13)) + "\n")
	body.WriteString(m.theme.muted().Render("SESSION  : ") + m.theme.indigo().Render("Remote Stream Launch") + "\n\n")
	body.WriteString(m.renderActionPath([]string{"WAKE", "WAIT", last}, innerWidth) + "\n")
	body.WriteString(m.cyberWave(innerWidth) + "\n\n")
	body.WriteString(m.theme.muted().Render("esc cancel   q quit"))

	card := renderCardBox(m.theme, "⚡ REMOTE LAUNCHER", body.String(), cardWidth, 0)
	centered := centerBlock(card, width)
	return "\n" + centered + "\n"
}

func (m *WakeModel) renderLoadingView(width int, mode LayoutMode) string {
	// 1. Error state (phaseLoadError)
	if m.phase == phaseLoadError {
		message := m.loadingError
		if message == "" {
			message = "The local inventory could not be read."
		}
		if width < 48 || m.height < 18 {
			var builder strings.Builder
			builder.WriteString("\n" + fitText(m.theme.title().Render("wol"), width) + "\n")
			builder.WriteString("\n" + fitText(m.theme.danger().Render("could not load"), width) + "\n\n")
			builder.WriteString(fitText(message, width) + "\n\n")
			builder.WriteString(fitText(m.theme.accent().Render("r retry")+m.theme.muted().Render("   q quit"), width) + "\n")
			return builder.String()
		}
		cardWidth := min(width-4, 60)
		innerWidth := cardWidth - 4
		var body strings.Builder
		body.WriteString("\n")
		body.WriteString(m.theme.danger().Render("could not load") + "\n\n")
		body.WriteString(fitText(message, innerWidth) + "\n\n")
		body.WriteString(m.theme.accent().Render("r retry") + m.theme.muted().Render("   q quit"))
		card := renderCardBox(m.theme, "⚠️  BOOT RECOVERY", body.String(), cardWidth, 0)
		centered := centerBlock(card, width)
		return "\n" + centered + "\n"
	}

	// 2. Normal loading phases (boot, refresh, check)
	title := "checking"
	badge := "SYSTEM INITIALIZING"
	if m.phase == phaseCheckingMachine {
		title = "checking power"
		badge = "PROBING POWER"
	} else if m.phase == phaseRefreshing {
		title = "refreshing"
		badge = "SYNCING FLEET"
	}

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

	controls := "q quit"
	if m.phase == phaseRefreshing || m.phase == phaseCheckingMachine {
		controls = "esc cancel   q quit"
	}

	// Compact mode for small terminals
	if width < 48 || m.height < 18 {
		var builder strings.Builder
		builder.WriteString("\n" + fitText(m.theme.title().Render("wol"), width) + "\n")
		builder.WriteString("\n" + fitText(m.theme.muted().Render(title), width) + "\n")
		if m.loadingTarget != "" {
			builder.WriteString(fitText(m.theme.accent().Render(m.loadingTarget), width) + "\n")
		}
		builder.WriteString("\n" + fitText(m.loadingSignal(width), width) + "\n\n")
		builder.WriteString(fitText(m.theme.accent().Render(stage), width) + "\n\n")
		builder.WriteString(fitText(m.theme.muted().Render(controls), width) + "\n")
		return builder.String()
	}

	// Modern Horizontally-Centered Cyber Telemetry HUD
	cardWidth := min(width-4, 62)
	innerWidth := cardWidth - 4
	spinners := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinner := "●"
	if m.motion.Enabled && !m.theme.ASCII {
		spinner = spinners[int(m.frame)%len(spinners)]
	}

	var body strings.Builder
	body.WriteString("\n")
	if m.loadingTarget != "" {
		body.WriteString(m.theme.muted().Render("TARGET  : ") + m.theme.cyan().Render(fitText(m.loadingTarget, innerWidth-12)) + "\n")
	} else if count > 0 {
		body.WriteString(m.theme.muted().Render("TARGET  : ") + m.theme.cyan().Render(fitText(fmt.Sprintf("Fleet (%d nodes)", count), innerWidth-12)) + "\n")
	} else {
		body.WriteString(m.theme.muted().Render("TARGET  : ") + m.theme.cyan().Render("Fleet Inventory") + "\n")
	}
	body.WriteString(m.theme.muted().Render("ACTION  : ") + m.theme.indigo().Render(fitText(title, innerWidth-12)) + "\n\n")

	body.WriteString(m.loadingSignal(innerWidth) + "\n")
	body.WriteString(m.cyberWave(innerWidth) + "\n\n")
	body.WriteString(m.theme.accent().Render(spinner+" "+stage) + "\n\n")
	body.WriteString(m.theme.muted().Render(controls))

	headerTitle := "◈ " + badge
	card := renderCardBox(m.theme, headerTitle, body.String(), cardWidth, 0)
	centered := centerBlock(card, width)
	return "\n" + centered + "\n"
}

func (m *WakeModel) cyberWave(width int) string {
	prefix := m.theme.muted().Render("SCAN   ")
	prefixW := 7
	suffix := m.theme.emerald().Render(" LIVE")
	suffixW := 5
	bracketW := 4 // "[ " + " ]"
	available := width - prefixW - suffixW - bracketW
	if available < 6 {
		return prefix + m.theme.cyan().Render("● ● ●") + suffix
	}
	waveWidth := min(available, 28)

	var wave strings.Builder
	if m.theme.ASCII {
		pattern := []byte{'-', '=', '+', '*', '#', '*', '+', '=', '-'}
		for i := 0; i < waveWidth; i++ {
			if m.motion.Enabled {
				wave.WriteByte(pattern[(int(m.frame)+i)%len(pattern)])
			} else {
				wave.WriteByte('-')
			}
		}
	} else {
		if m.motion.Enabled {
			bars := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█', '▇', '▆', '▅', '▄', '▃', '▂'}
			for i := 0; i < waveWidth; i++ {
				idx := (int(m.frame) + i) % len(bars)
				wave.WriteRune(bars[idx])
			}
		} else {
			for i := 0; i < waveWidth; i++ {
				wave.WriteRune('▄')
			}
		}
	}

	leftB := m.theme.cardLine().Render("[ ")
	rightB := m.theme.cardLine().Render(" ]")
	return prefix + leftB + m.theme.cyan().Render(wave.String()) + rightB + suffix
}

func centerBlock(block string, totalWidth int) string {
	lines := strings.Split(block, "\n")
	maxW := 0
	for _, l := range lines {
		w := lipgloss.Width(stripANSI(l))
		if w > maxW {
			maxW = w
		}
	}
	if maxW == 0 || maxW >= totalWidth {
		return block
	}
	leftPad := (totalWidth - maxW) / 2
	pad := strings.Repeat(" ", leftPad)
	var sb strings.Builder
	for i, l := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if strings.TrimSpace(l) != "" {
			sb.WriteString(pad)
			sb.WriteString(l)
		}
	}
	return sb.String()
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
