package ui

import (
	"path"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/notes"
)

type Mode uint8

const (
	Browse Mode = iota
	NewNotePrompt
	NewDirectoryPrompt
	RenamePrompt
	CopyPrompt
	MovePrompt
	SearchPrompt
	TrashConfirm
	PermanentDeleteConfirm
	ThemePicker
	RenderStylePicker
	CommandPalette
)

type mutationResult struct {
	path notes.RelPath
	err  error
}

func newPrompt(mode Mode, selected notes.Entry, snapshot notes.Snapshot) textinput.Model {
	input := textinput.New()
	input.Prompt = "> "
	if mode == RenamePrompt {
		input.SetValue(selected.Name)
	} else if mode == CopyPrompt || mode == MovePrompt {
		input.ShowSuggestions = true
		suggestions := []string{}
		if selected.Kind != notes.KindDirectory || parentPath(selected.Path) != "" {
			suggestions = append(suggestions, path.Join("", selected.Name))
		}
		for _, entry := range snapshot.Entries {
			if entry.Kind == notes.KindDirectory {
				if selected.Kind == notes.KindDirectory && (entry.Path == selected.Path || strings.HasPrefix(string(entry.Path), string(selected.Path)+"/")) {
					continue
				}
				suggestions = append(suggestions, path.Join(string(entry.Path), selected.Name))
			}
		}
		input.SetSuggestions(suggestions)
	}
	input.Focus()
	return input
}

func (m Model) updatePrompt(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		m.closeModal()
		return m, nil
	case "enter":
		value := m.input.Value()
		if strings.TrimSpace(value) == "" {
			m.modalError = "name is required"
			return m, nil
		}
		mode, entry := m.mode, m.modalEntry
		m.closeModal()
		switch mode {
		case NewNotePrompt:
			return m, m.createNoteCommand(parentForCreate(entry), value)
		case NewDirectoryPrompt:
			return m, m.createDirectoryCommand(parentForCreate(entry), value)
		case RenamePrompt:
			return m, m.renameCommand(entry, value)
		case CopyPrompt:
			return m, m.copyCommand(entry, notes.RelPath(value))
		case MovePrompt:
			return m, m.moveCommand(entry, notes.RelPath(value))
		}
	}
	m.modalError = ""
	if message.String() == "alt+backspace" && (m.mode == CopyPrompt || m.mode == MovePrompt) {
		value := []rune(m.input.Value())
		position := min(m.input.Position(), len(value))
		slash := -1
		for index := position - 1; index >= 0; index-- {
			if value[index] == '/' {
				slash = index
				break
			}
		}
		m.input.SetValue(string(append(value[:max(0, slash)], value[position:]...)))
		m.input.SetCursor(max(0, slash))
		return m, nil
	}
	if message.String() == "tab" && (m.mode == CopyPrompt || m.mode == MovePrompt) {
		m.input.SetValue(completePath(m.input.Value(), m.input.MatchedSuggestions()))
		m.input.CursorEnd()
		return m, nil
	}
	m.input, _ = m.input.Update(message)
	return m, nil
}

func completePath(value string, suggestions []string) string {
	if len(suggestions) == 0 {
		return value
	}
	common := []rune(suggestions[0])
	for _, suggestion := range suggestions[1:] {
		candidate := []rune(suggestion)
		index := 0
		for index < min(len(common), len(candidate)) && common[index] == candidate[index] {
			index++
		}
		common = common[:index]
	}
	valueLength := len([]rune(value))
	if len(common) <= valueLength {
		return value
	}
	for index, character := range common[valueLength:] {
		if character == '/' {
			return string(common[:valueLength+index+1])
		}
	}
	return string(common)
}

func (m Model) promptTitle() string {
	return map[Mode]string{
		NewNotePrompt:      "New note",
		NewDirectoryPrompt: "New directory",
		RenamePrompt:       "Rename " + m.modalEntry.Name,
		CopyPrompt:         "Copy " + m.modalEntry.Name,
		MovePrompt:         "Move " + m.modalEntry.Name,
	}[m.mode]
}

func (m Model) promptStatus(width int) (string, int, int) {
	prefix := m.promptTitle() + ": "
	suffix := ""
	suggestion := m.input.CurrentSuggestion()
	if suggestion == m.input.Value() {
		suggestion = ""
	}
	if suggestion != "" {
		suffix += "  " + suggestion
	}
	if m.modalError != "" {
		suffix += "  ! " + m.modalError
	}
	line, visibleSuffix := statusInputLineWithSuffix(prefix, m.input, suffix, width)
	if suggestion == "" || !strings.HasPrefix(visibleSuffix, "  ") {
		return line, 0, 0
	}
	start := ansi.StringWidth(line) - ansi.StringWidth(visibleSuffix) + 2
	end := start + min(ansi.StringWidth(suggestion), ansi.StringWidth(visibleSuffix)-2)
	return line, start, end
}

func statusInputLine(prefix string, input textinput.Model, suffix string, width int) string {
	line, _ := statusInputLineWithSuffix(prefix, input, suffix, width)
	return line
}

func statusInputLineWithSuffix(prefix string, input textinput.Model, suffix string, width int) (string, string) {
	inputMinimum := min(12, max(1, width/3))
	prefix = ansi.Truncate(prefix, max(0, width-inputMinimum), "")
	suffix = ansi.Truncate(suffix, max(0, width-ansi.StringWidth(prefix)-inputMinimum), "")
	return prefix + statusInput(input, max(1, width-ansi.StringWidth(prefix)-ansi.StringWidth(suffix))) + suffix, suffix
}

func statusInput(input textinput.Model, width int) string {
	value := []rune(input.Value())
	position := min(max(0, input.Position()), len(value))
	promptWidth := ansi.StringWidth(input.Prompt)
	available := max(0, width-promptWidth-1)
	left, right := string(value[:position]), string(value[position:])
	leftWidth, rightWidth := ansi.StringWidth(left), ansi.StringWidth(right)
	leftBudget, rightBudget := available/2, available-available/2
	if rightWidth < rightBudget {
		leftBudget += rightBudget - rightWidth
		rightBudget = rightWidth
	}
	if leftWidth < leftBudget {
		rightBudget += leftBudget - leftWidth
		leftBudget = leftWidth
	}
	left = ansi.Cut(left, max(0, leftWidth-leftBudget), leftWidth)
	right = ansi.Truncate(right, rightBudget, "")
	return ansi.Truncate(input.Prompt, max(0, width-1), "") + left + "▏" + right
}

func isTextPrompt(mode Mode) bool {
	return mode == NewNotePrompt || mode == NewDirectoryPrompt || mode == RenamePrompt || mode == CopyPrompt || mode == MovePrompt
}

func isConfirmation(mode Mode) bool {
	return mode == TrashConfirm || mode == PermanentDeleteConfirm
}

func (m *Model) closeModal() {
	m.mode = Browse
	m.input.Blur()
	m.modalEntry = notes.Entry{}
	m.modalError = ""
	m.tree = m.tree.SetViewport(paneHeight(m.height, m.statusBarVisible()))
}

func parentForCreate(entry notes.Entry) notes.RelPath {
	if entry.Path == "" {
		return ""
	}
	if entry.Kind == notes.KindDirectory {
		return entry.Path
	}
	return parentPath(entry.Path)
}

func (m Model) createNoteCommand(parent notes.RelPath, title string) tea.Cmd {
	return func() tea.Msg {
		document, err := m.store.CreateNote(m.ctx, parent, title)
		return mutationResult{path: document.Path, err: err}
	}
}

func (m Model) createDirectoryCommand(parent notes.RelPath, name string) tea.Cmd {
	return func() tea.Msg {
		entry, err := m.store.CreateDirectory(m.ctx, parent, name)
		return mutationResult{path: entry.Path, err: err}
	}
}

func (m Model) renameCommand(entry notes.Entry, name string) tea.Cmd {
	return func() tea.Msg {
		path, err := m.store.Rename(m.ctx, entry.Path, name, entry.Identity)
		return mutationResult{path: path, err: err}
	}
}

func (m Model) copyCommand(entry notes.Entry, destination notes.RelPath) tea.Cmd {
	return func() tea.Msg {
		path, err := m.store.Copy(m.ctx, entry.Path, destination, entry.Identity)
		return mutationResult{path: path, err: err}
	}
}

func (m Model) moveCommand(entry notes.Entry, destination notes.RelPath) tea.Cmd {
	return func() tea.Msg {
		path, err := m.store.Move(m.ctx, entry.Path, destination, entry.Identity)
		return mutationResult{path: path, err: err}
	}
}

func (m Model) deleteCommand(entry notes.Entry) tea.Cmd {
	return func() tea.Msg {
		err := m.store.Delete(m.ctx, entry.Path, entry.Identity)
		if err != nil {
			return mutationResult{err: err}
		}
		return mutationResult{path: parentPath(entry.Path)}
	}
}
