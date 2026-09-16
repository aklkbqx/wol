package tui

import "time"

type Stage uint8

const (
	StageIdle Stage = iota
	StageSelect
	StageSignal
)

// Motion is a tiny clock used only while a state transition is visible.
type Motion struct {
	Enabled  bool
	Frame    uint64
	Started  time.Time
	Duration time.Duration
	Until    time.Time
	Stage    Stage
	Origin   int
	Dest     int
}

func NewMotion(enabled bool) Motion {
	return Motion{Enabled: enabled}
}

func (m *Motion) Trigger(now time.Time, duration time.Duration) {
	m.TriggerStage(now, StageSignal, duration, 0, 0)
}

func (m *Motion) TriggerStage(now time.Time, stage Stage, duration time.Duration, origin, dest int) {
	if !m.Enabled {
		return
	}
	if duration <= 0 {
		duration = 800 * time.Millisecond
	}
	m.Stage, m.Origin, m.Dest = stage, origin, dest
	m.Started = now
	m.Duration = duration
	m.Until = now.Add(duration)
}

func (m Motion) Active(now time.Time) bool {
	return m.Enabled && m.Stage != StageIdle && !m.Until.IsZero() && now.Before(m.Until)
}

func (m Motion) T(now time.Time) float64 {
	if m.Duration <= 0 || m.Started.IsZero() {
		return 1
	}
	elapsed := now.Sub(m.Started)
	if elapsed <= 0 {
		return 0
	}
	if elapsed >= m.Duration {
		return 1
	}
	return float64(elapsed) / float64(m.Duration)
}

func (m *Motion) Step(now time.Time) bool {
	if !m.Active(now) {
		return false
	}
	m.Frame++
	return true
}

func (m Motion) TickInterval() time.Duration {
	switch m.Stage {
	case StageSelect:
		return 40 * time.Millisecond
	case StageSignal:
		return 33 * time.Millisecond
	default:
		return 0
	}
}
