package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/gonfff/jotmd/internal/editor"
	"github.com/gonfff/jotmd/internal/notes"
)

type editorFinished struct{ err error }

func (m *Model) editSelected() tea.Cmd {
	entry, ok := m.tree.Selected()
	if !ok || entry.Kind != notes.KindMarkdown {
		return nil
	}
	if m.editorCommand.Executable == "" {
		m.status = "editor: not configured"
		return nil
	}
	path, err := m.store.AbsolutePath(entry.Path)
	if err != nil {
		m.status = fmt.Sprintf("editor: %v", err)
		return nil
	}
	return editor.Exec(m.editorCommand, path, func(err error) tea.Msg {
		return editorFinished{err: err}
	})
}
