package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) helpView(width, height int) string {
	lines := m.helpLines(width)
	contentHeight := max(1, height-1)
	start := min(max(0, m.helpOffset), max(0, len(lines)-contentHeight))
	footer := "↑/↓ scroll"
	for _, binding := range m.bindings {
		if binding.Action == ActionClose && hasContext(binding.Contexts, ContextHelp) && len(binding.Keys) != 0 {
			footer += "  " + strings.Join(binding.Keys, "/") + " close"
			break
		}
	}
	lines = append(paddedLines(lines[start:], width, contentHeight), lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Muted)).Render(footer))
	return popupLines(lines, width, height)
}

func (m Model) helpLines(width int) []string {
	titles := []string{"Navigation", "Notes", "Preview", "Application"}
	type hint struct{ keys, label string }
	groups := make([][]hint, len(titles))
	for _, binding := range m.bindings {
		if len(binding.Keys) != 0 && (hasContext(binding.Contexts, ContextTree) || hasContext(binding.Contexts, ContextPreview)) {
			group := 3
			switch {
			case strings.HasPrefix(binding.Name, "tree."):
				group = 0
			case strings.HasPrefix(binding.Name, "note."), strings.HasPrefix(binding.Name, "directory."):
				group = 1
			case strings.HasPrefix(binding.Name, "preview."):
				group = 2
			}
			groups[group] = append(groups[group], hint{keys: strings.Join(binding.Keys, "/"), label: binding.Label})
		}
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Heading))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Foreground))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.Palette.Muted))
	columnWidth := max(1, (width-2)/2)
	blocks := make([][]string, len(groups))
	for index, group := range groups {
		keyWidth := 0
		for _, row := range group {
			keyWidth = max(keyWidth, ansi.StringWidth(row.keys))
		}
		blocks[index] = []string{titleStyle.Render(titles[index])}
		for _, row := range group {
			keys := row.keys + strings.Repeat(" ", keyWidth-ansi.StringWidth(row.keys))
			line := "  " + keyStyle.Render(keys) + "  " + labelStyle.Render(row.label)
			blocks[index] = append(blocks[index], line)
		}
	}

	columns := [][]string{
		append(append(blocks[0], ""), blocks[2]...),
		append(append(blocks[1], ""), blocks[3]...),
	}
	rows := max(len(columns[0]), len(columns[1]))
	lines := make([]string, 0, rows)
	for row := range rows {
		parts := make([]string, 0, len(columns))
		for column, entries := range columns {
			part := ""
			if row < len(entries) {
				part = ansi.Truncate(entries[row], columnWidth, "")
			}
			if column+1 < len(columns) {
				part += strings.Repeat(" ", max(0, columnWidth-ansi.StringWidth(part)))
			}
			parts = append(parts, part)
		}
		lines = append(lines, strings.Join(parts, "  "))
	}
	return lines
}

func (m Model) scrollHelp(delta int) Model {
	width, height := m.popupSize()
	m.helpOffset = min(max(0, len(m.helpLines(width))-max(1, height-1)), max(0, m.helpOffset+delta))
	return m
}
