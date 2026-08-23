package tui

import "strings"

// stripANSI removes SGR terminal escapes for width checks and plain-text
// assertions. Wake Desk labels are generated internally, so a conservative
// parser is preferable to trying to interpret arbitrary control sequences.
func stripANSI(value string) string {
	var builder strings.Builder
	inEscape := false
	for i := 0; i < len(value); i++ {
		if value[i] == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if value[i] == 'm' {
				inEscape = false
			}
			continue
		}
		builder.WriteByte(value[i])
	}
	return builder.String()
}
