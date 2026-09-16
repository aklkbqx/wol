package ui

// Night is the Wake Desk hex vocabulary.
// These names must not reuse ColorPrimary/ColorMuted/ColorDanger from theme.go.
var Night = struct {
	Ink, Panel, Paper, Muted, Line, Amber, Live, Danger string
}{
	Ink:    "#10140F",
	Panel:  "#181E16",
	Paper:  "#D8D2C3",
	Muted:  "#7A7F74",
	Line:   "#2A3128",
	Amber:  "#C9842A",
	Live:   "#4F8F78",
	Danger: "#C45C4A",
}

// DeskCSS is the shared :root + box-sizing + reduced-motion kill switch.
// pageLocal must not repeat these three rules.
func DeskCSS() string {
	return `:root{color-scheme:dark;--ink:` + Night.Ink +
		`;--paper:` + Night.Paper + `;--muted:` + Night.Muted +
		`;--line:` + Night.Line + `;--amber:` + Night.Amber +
		`;--live:` + Night.Live + `;--danger:` + Night.Danger +
		`;--panel:` + Night.Panel + `}` +
		`*{box-sizing:border-box}` +
		`@media(prefers-reduced-motion:reduce){*{animation:none!important;transition:none!important}}`
}
