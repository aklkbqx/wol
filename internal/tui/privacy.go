package tui

import "strings"

// privateField is presentation-only: action inputs and stored values stay intact.
func (m *WakeModel) privateField(value string) string {
	if m.redactIdentifiers && value != "" {
		return "[hidden]"
	}
	return value
}

// Free-form diagnostics may contain endpoints or paths not in the inventory.
// Hide the entire detail rather than relying on an incomplete identifier list.
// Success/power indicators are rendered separately from actual model state.
func (m *WakeModel) privateMessage(value string) string {
	if !m.redactIdentifiers || value == "" {
		return value
	}
	lower := strings.ToLower(value)
	if strings.HasSuffix(lower, "cancelled.") || strings.HasSuffix(lower, "canceled.") || lower == "cancelled" {
		return "Action cancelled."
	}
	if strings.Contains(lower, "failed:") || strings.HasSuffix(lower, "failed") {
		return "Operation failed. Details hidden."
	}
	return "Details hidden."
}
