package tui

import "time"

// Motion is a tiny frame clock used only while a state transition is visible.
// It prevents the old always-on 300ms redraw loop from burning CPU when idle.
type Motion struct {
	Enabled bool
	Frame   uint64
	Until   time.Time
}

func NewMotion(enabled bool) Motion {
	return Motion{Enabled: enabled}
}

func (m *Motion) Trigger(now time.Time, duration time.Duration) {
	if !m.Enabled {
		return
	}
	if duration <= 0 {
		duration = 700 * time.Millisecond
	}
	m.Until = now.Add(duration)
}

func (m Motion) Active(now time.Time) bool {
	return m.Enabled && !m.Until.IsZero() && now.Before(m.Until)
}

func (m *Motion) Step(now time.Time) bool {
	if !m.Active(now) {
		return false
	}
	m.Frame++
	return true
}
