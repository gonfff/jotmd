package ui

import (
	"cmp"
	"slices"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/notes"
)

type paletteItem struct {
	binding Binding
	enabled bool
	reason  string
}

type paletteState struct {
	origin   Mode
	input    textinput.Model
	items    []paletteItem
	selected int
}

func (m *Model) openPalette() {
	input := textinput.New()
	input.Prompt = ": "
	input.Focus()
	m.palette = paletteState{origin: m.mode, input: input}
	m.mode = CommandPalette
	m.refreshPalette()
}

func (m *Model) closePalette() {
	m.palette.input.Blur()
	m.mode = m.palette.origin
	m.palette.items = nil
	m.palette.selected = 0
}

func (m *Model) refreshPalette() {
	items := make([]paletteItem, 0, len(m.bindings))
	for _, binding := range m.bindings {
		if !paletteBinding(binding) {
			continue
		}
		enabled, reason := m.paletteActionState(binding.Action)
		items = append(items, paletteItem{binding: binding, enabled: enabled, reason: reason})
	}
	slices.SortFunc(items, func(left, right paletteItem) int {
		if order := cmp.Compare(left.binding.Label, right.binding.Label); order != 0 {
			return order
		}
		return cmp.Compare(left.binding.Name, right.binding.Name)
	})
	query := strings.ToLower(m.palette.input.Value())
	if query != "" {
		targets := make([]string, len(items))
		for index, item := range items {
			targets[index] = strings.ToLower(item.binding.Label + " " + item.binding.Name)
		}
		ranks := list.DefaultFilter(query, targets)
		filtered := make([]paletteItem, len(ranks))
		for index, rank := range ranks {
			filtered[index] = items[rank.Index]
		}
		items = filtered
	}
	m.palette.items = items
	m.palette.selected = min(m.palette.selected, max(0, len(items)-1))
}

func paletteBinding(binding Binding) bool {
	if binding.Action == ActionPalette || !hasAnyContext(binding.Contexts, ContextTree, ContextPreview, ContextSearch) {
		return false
	}
	switch binding.Action {
	case ActionUp, ActionDown, ActionFirst, ActionLast, ActionCollapse, ActionExpand, ActionParent, ActionOpen,
		ActionClose, ActionThemeUp, ActionThemeDown, ActionThemeApply, ActionThemeClose:
		return false
	}
	return true
}

func hasAnyContext(contexts []Context, wants ...Context) bool {
	for _, want := range wants {
		if hasContext(contexts, want) {
			return true
		}
	}
	return false
}

func (m Model) paletteActionState(action Action) (bool, string) {
	entry, selected := m.tree.Selected()
	switch action {
	case ActionEdit:
		if !selected {
			return false, "select a note"
		}
		if entry.Kind != notes.KindMarkdown {
			return false, "select a note"
		}
		if m.editorCommand.Executable == "" {
			return false, "editor not configured"
		}
	case ActionRename, ActionTrash, ActionDelete:
		if !selected {
			return false, "select an entry"
		}
	case ActionCopy, ActionMove:
		if !selected || entry.Kind != notes.KindMarkdown {
			return false, "select a note"
		}
	}
	return true, ""
}

func (m Model) updatePalette(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		m.closePalette()
		return m, nil
	case "up", "ctrl+p":
		if m.palette.selected > 0 {
			m.palette.selected--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.palette.selected+1 < len(m.palette.items) {
			m.palette.selected++
		}
		return m, nil
	case "enter":
		if len(m.palette.items) == 0 {
			return m, nil
		}
		item := m.palette.items[m.palette.selected]
		if !item.enabled {
			return m, nil
		}
		origin := m.palette.origin
		m.closePalette()
		if origin == SearchPrompt {
			m.closeSearch()
		}
		return m.dispatchAction(item.binding.Action)
	}
	m.palette.input, _ = m.palette.input.Update(message)
	m.refreshPalette()
	return m, nil
}

func (m Model) paletteView(width, height int) string {
	lines := []string{"Commands", "", m.palette.input.View()}
	available := max(1, height-len(lines)-2)
	start := 0
	if m.palette.selected >= available {
		start = m.palette.selected - available + 1
	}
	for index := start; index < len(m.palette.items) && index < start+available; index++ {
		item := m.palette.items[index]
		line := "  " + item.binding.Label + "  " + strings.Join(item.binding.Keys, "/")
		if !item.enabled {
			line += "  (" + item.reason + ")"
			line = lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Muted)).Render(line)
		}
		if index == m.palette.selected {
			line = lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.SelectionForeground)).Background(lipgloss.Color(m.theme.Palette.SelectionBackground)).Render("> " + strings.TrimPrefix(line, "  "))
		}
		lines = append(lines, ansi.Truncate(line, width, ""))
	}
	if len(m.palette.items) == 0 {
		lines = append(lines, "No actions.")
	}
	lines = append(lines, "", "Enter run  ↑/↓ navigate  Esc cancel")
	return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Accent)).Render(popupLines(lines, width, height))
}
