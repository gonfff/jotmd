package ui

import (
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gonfff/jotmd/internal/theme"
)

type themePickerState struct {
	input    textinput.Model
	filtered []int
	selected int
	original theme.Theme
}

func (m *Model) openThemePicker() {
	m.openThemePickerWithQuery("")
}

func (m *Model) openThemePickerWithQuery(query string) {
	m.mode = ThemePicker
	m.themePicker = themePickerState{input: pickerInput(query), original: m.theme}
	m.refreshThemePicker()
	found := false
	for index, candidate := range m.themes {
		if candidate.Name == m.theme.Name {
			found = m.themePicker.selectSource(index)
			break
		}
	}
	if query != "" && !found {
		m.previewThemePicker()
	}
	m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
}

func (m *Model) updateThemePicker(action Action) pickerResult {
	switch action {
	case ActionThemeClose:
		m.theme = m.themePicker.original
		m.themePicker.input.Blur()
		m.mode = Browse
		m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
		return pickerResult{render: true}
	case ActionThemeApply:
		if len(m.themePicker.filtered) == 0 {
			return pickerResult{}
		}
		m.themePicker.input.Blur()
		m.mode = Browse
		m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
		return pickerResult{}
	case ActionThemeDown:
		if m.themePicker.selected+1 < len(m.themePicker.filtered) {
			m.themePicker.selected++
		} else {
			return pickerResult{}
		}
	case ActionThemeUp:
		if m.themePicker.selected > 0 {
			m.themePicker.selected--
		} else {
			return pickerResult{}
		}
	default:
		return pickerResult{}
	}
	m.previewThemePicker()
	return pickerResult{render: true}
}

func (m *Model) updateThemePickerInput(message tea.KeyPressMsg) pickerResult {
	m.themePicker.input, _ = m.themePicker.input.Update(message)
	m.refreshThemePicker()
	m.previewThemePicker()
	return pickerResult{render: true}
}

func (m *Model) refreshThemePicker() {
	values := make([]string, len(m.themes))
	for index, candidate := range m.themes {
		values[index] = candidate.Name
	}
	m.themePicker.filtered = filterIndices(m.themePicker.input.Value(), values)
	m.themePicker.selected = min(m.themePicker.selected, max(0, len(m.themePicker.filtered)-1))
}

func (m *Model) previewThemePicker() {
	if len(m.themePicker.filtered) != 0 {
		m.theme = m.themes[m.themePicker.filtered[m.themePicker.selected]]
	}
}

func (p *themePickerState) selectSource(source int) bool {
	for index, candidate := range p.filtered {
		if candidate == source {
			p.selected = index
			return true
		}
	}
	return false
}

type pickerResult struct{ render bool }

func pickerThemes(current theme.Theme, themes []theme.Theme, noColor bool) []theme.Theme {
	valid := make([]theme.Theme, 0, len(themes))
	for _, candidate := range themes {
		if theme.Dump(io.Discard, candidate) == nil {
			if noColor {
				candidate.Palette = theme.Palette{}
			}
			valid = append(valid, candidate)
		}
	}
	if len(valid) == 0 {
		return []theme.Theme{current}
	}
	return valid
}

func (m Model) themePickerView(width, height int) string {
	lines := make([]string, 0, height)
	available := max(1, height)
	start := 0
	if m.themePicker.selected >= available {
		start = m.themePicker.selected - available + 1
	}
	for index := start; index < len(m.themePicker.filtered) && index < start+available; index++ {
		candidate := m.themes[m.themePicker.filtered[index]]
		prefix := "  "
		if index == m.themePicker.selected {
			prefix = "> "
		}
		lines = append(lines, prefix+candidate.Name)
	}
	if len(lines) == 0 && m.themePicker.input.Value() != "" {
		lines = append(lines, "No matches.")
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Accent)).Render(popupLines(lines, width, height))
}

func pickerInput(value string) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.SetValue(value)
	input.Focus()
	return input
}

func filterIndices(query string, values []string) []int {
	if query == "" {
		indices := make([]int, len(values))
		for index := range values {
			indices[index] = index
		}
		return indices
	}
	filtered := make([]string, len(values))
	for index, value := range values {
		filtered[index] = strings.ToLower(value)
	}
	ranks := list.DefaultFilter(strings.ToLower(query), filtered)
	indices := make([]int, len(ranks))
	for index, rank := range ranks {
		indices[index] = rank.Index
	}
	return indices
}
