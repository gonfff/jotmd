package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gonfff/jotmd/internal/theme"
)

type renderStylePickerState struct {
	input    textinput.Model
	filtered []int
	selected int
	original string
}

func (m *Model) openRenderStylePicker() {
	m.openRenderStylePickerWithQuery("")
}

func (m *Model) openRenderStylePickerWithQuery(query string) {
	styles := theme.RenderStyles()
	m.mode = RenderStylePicker
	m.renderStylePicker = renderStylePickerState{input: pickerInput(query), original: m.preview.renderStyle}
	m.refreshRenderStylePicker()
	found := false
	for index, style := range styles {
		if style == m.preview.renderStyle {
			found = m.renderStylePicker.selectSource(index)
			break
		}
	}
	if query != "" && !found && len(m.renderStylePicker.filtered) != 0 {
		m.preview.SetRenderStyle(styles[m.renderStylePicker.filtered[m.renderStylePicker.selected]])
	}
	m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
}

func (m *Model) updateRenderStylePicker(action Action) pickerResult {
	styles := theme.RenderStyles()
	switch action {
	case ActionThemeClose:
		m.preview.SetRenderStyle(m.renderStylePicker.original)
		m.renderStylePicker.input.Blur()
		m.mode = Browse
		m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
		return pickerResult{render: true}
	case ActionThemeApply:
		if len(m.renderStylePicker.filtered) == 0 {
			return pickerResult{}
		}
		m.renderStylePicker.input.Blur()
		m.mode = Browse
		m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
		return pickerResult{}
	case ActionThemeDown:
		if m.renderStylePicker.selected+1 >= len(m.renderStylePicker.filtered) {
			return pickerResult{}
		}
		m.renderStylePicker.selected++
	case ActionThemeUp:
		if m.renderStylePicker.selected == 0 {
			return pickerResult{}
		}
		m.renderStylePicker.selected--
	default:
		return pickerResult{}
	}
	m.preview.SetRenderStyle(styles[m.renderStylePicker.filtered[m.renderStylePicker.selected]])
	return pickerResult{render: true}
}

func (m *Model) updateRenderStylePickerInput(message tea.KeyPressMsg) pickerResult {
	m.renderStylePicker.input, _ = m.renderStylePicker.input.Update(message)
	m.refreshRenderStylePicker()
	if len(m.renderStylePicker.filtered) != 0 {
		m.preview.SetRenderStyle(theme.RenderStyles()[m.renderStylePicker.filtered[m.renderStylePicker.selected]])
	}
	return pickerResult{render: true}
}

func (m *Model) refreshRenderStylePicker() {
	m.renderStylePicker.filtered = filterIndices(m.renderStylePicker.input.Value(), theme.RenderStyles())
	m.renderStylePicker.selected = min(m.renderStylePicker.selected, max(0, len(m.renderStylePicker.filtered)-1))
}

func (p *renderStylePickerState) selectSource(source int) bool {
	for index, candidate := range p.filtered {
		if candidate == source {
			p.selected = index
			return true
		}
	}
	return false
}

func (m Model) renderStylePickerView(width, height int) string {
	styles := theme.RenderStyles()
	lines := make([]string, 0, len(styles))
	for index, source := range m.renderStylePicker.filtered {
		style := styles[source]
		prefix := "  "
		if index == m.renderStylePicker.selected {
			prefix = "> "
		}
		lines = append(lines, prefix+style)
	}
	if len(lines) == 0 && m.renderStylePicker.input.Value() != "" {
		lines = append(lines, "No matches.")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Accent)).Render(popupLines(lines, width, height))
}
