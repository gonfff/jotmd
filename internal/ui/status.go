package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) statusView(width int, context Context) string {
	if isConfirmation(m.mode) {
		return m.renderStatusLine(m.confirmationStatus(), width)
	}
	if isTextPrompt(m.mode) {
		line, suggestionStart, suggestionEnd := m.promptStatus(width)
		line = m.renderStatusLine(line, width)
		if suggestionStart < suggestionEnd {
			line = lipgloss.StyleRanges(line, lipgloss.NewRange(suggestionStart, suggestionEnd, lipgloss.NewStyle().
				Foreground(lipgloss.Color(m.theme.Palette.Muted)).
				Background(lipgloss.Color(m.theme.Palette.Status))))
		}
		return line
	}
	if m.mode == SearchPrompt {
		return m.renderStatusLine(m.searchStatus(width), width)
	}
	if m.mode == ThemePicker {
		return m.renderStatusLine(statusInputLine("Theme: ", m.themePicker.input, "", width), width)
	}
	if m.mode == RenderStylePicker {
		return m.renderStatusLine(statusInputLine("Render style: ", m.renderStylePicker.input, "", width), width)
	}
	diagnostic := m.status
	if m.watchWarning != "" && !strings.Contains(diagnostic, m.watchWarning) {
		if diagnostic != "" {
			diagnostic += "; "
		}
		diagnostic += m.watchWarning
	}
	if m.configWarning != "" && !strings.Contains(diagnostic, m.configWarning) {
		if diagnostic != "" {
			diagnostic += "; "
		}
		diagnostic += m.configWarning
	}
	line := ""
	if selected, ok := m.tree.Selected(); ok {
		line = string(selected.Path)
	}
	if diagnostic != "" {
		if line != "" {
			line += "  "
		}
		line += "! " + diagnostic
	}
	footer := ""
	for _, binding := range m.bindings {
		if binding.Action == ActionHelp && hasContext(binding.Contexts, context) && len(binding.Keys) != 0 {
			footer = binding.Keys[0] + " " + binding.Label
			break
		}
	}
	if footerWidth := ansi.StringWidth(footer); footer != "" && footerWidth <= width {
		rightPadding := min(2, max(0, width-footerWidth))
		contentWidth := width - rightPadding
		line = ansi.Truncate(line, max(0, contentWidth-footerWidth-2), "")
		line += strings.Repeat(" ", max(0, contentWidth-ansi.StringWidth(line)-footerWidth)) + footer + strings.Repeat(" ", rightPadding)
	} else {
		line = ansi.Truncate(line, width, "")
		line += strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
	}
	return m.renderStatusLine(line, width)
}

func (m Model) statusBarVisible() bool {
	return m.cfg.StatusBar || m.configWarning != "" || isConfirmation(m.mode) || isTextPrompt(m.mode) || m.mode == SearchPrompt || m.mode == ThemePicker || m.mode == RenderStylePicker
}

func (m Model) renderStatusLine(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	line += strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Palette.SelectionForeground)).
		Background(lipgloss.Color(m.theme.Palette.Status)).
		Render(line)
}

func (m Model) footer(width int, context Context) string {
	parts := make([]string, 0)
	for _, binding := range m.bindings {
		if hasContext(binding.Contexts, context) && len(binding.Keys) != 0 {
			part := binding.Keys[0] + " " + binding.Label
			if width >= 0 && ansi.StringWidth(strings.Join(append(parts, part), "  ")) > width {
				break
			}
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "  ")
}

func bindingHint(binding Binding) string {
	return strings.Join(binding.Keys, "/") + "  " + binding.Label
}
