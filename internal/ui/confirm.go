package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/gonfff/jotmd/internal/notes"
)

func (m Model) updateConfirmation(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch physicalKey(message).String() {
	case "y", "enter":
		entry, mode := m.modalEntry, m.mode
		m.closeModal()
		if mode == TrashConfirm {
			return m, m.trashCommand(entry)
		}
		return m, m.deleteCommand(entry)
	case "esc":
		m.closeModal()
		return m, nil
	}
	return m, nil
}

func (m Model) confirmationStatus() string {
	if m.mode == TrashConfirm {
		return "Move " + m.modalEntry.Name + " to Trash?  y/Enter confirm  Esc cancel"
	}
	return "Permanently delete " + m.modalEntry.Name + "?  y/Enter confirm  Esc cancel"
}

func (m Model) trashCommand(entry notes.Entry) tea.Cmd {
	return func() tea.Msg {
		err := m.store.Trash(m.ctx, entry.Path, entry.Identity)
		if err != nil {
			return mutationResult{err: err}
		}
		return mutationResult{path: parentPath(entry.Path)}
	}
}
