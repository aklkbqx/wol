package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderBox renders a structured information card with key-value pairs
func RenderBox(title string, rows [][]string) string {
	maxKeyLen := 0
	for _, row := range rows {
		if len(row) > 0 && len(row[0]) > maxKeyLen {
			maxKeyLen = len(row[0])
		}
	}

	var sb strings.Builder
	titleStyled := StyleTitle.Render(" ✨ " + strings.ToUpper(title) + " ")
	sb.WriteString(titleStyled)
	sb.WriteString("\n\n")

	for _, row := range rows {
		if len(row) == 0 {
			sb.WriteString("\n")
			continue
		}
		key := row[0]
		val := ""
		if len(row) > 1 {
			val = row[1]
		}
		paddedKey := key + strings.Repeat(" ", maxKeyLen-len(key))
		sb.WriteString(fmt.Sprintf("  %s  %s  %s\n", StyleKey.Render(paddedKey), StyleMuted.Render("│"), val))
	}

	return StyleBox.Render(sb.String())
}

// RenderHeader renders a top-level banner
func RenderHeader(title string, subtext string) string {
	titleBlock := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#ffffff")).
		Background(lipgloss.Color("#7c3aed")).
		Padding(0, 2).
		Render("✨ " + strings.ToUpper(title) + " ✨")

	sub := StyleMuted.Render(subtext)
	return fmt.Sprintf("\n %s  %s\n", titleBlock, sub)
}

// RenderCard renders a simple framed card
func RenderCard(title string, content string) string {
	header := StyleTitle.Render(" ◈ " + title)
	body := fmt.Sprintf("%s\n\n%s", header, content)
	return StyleBox.Render(body)
}
