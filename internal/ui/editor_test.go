package ui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/editor"
	"github.com/gonfff/jotmd/internal/notes"
)

func TestEditActionUsesDefaultAndConfiguredBindings(t *testing.T) {
	if action, ok := actionForKey(DefaultBindings(), "e", ContextTree); !ok || action != ActionEdit {
		t.Fatalf("actionForKey(e) = (%q, %t), want note edit", action, ok)
	}
	registry := ActionRegistry()
	for _, action := range registry {
		if action.Name == "note.edit" {
			bindings := BindingsForKeymap(config.Keymap{Bindings: []config.Binding{{Action: "note.edit", Keys: []string{"x"}}}})
			if got, ok := actionForKey(bindings, "x", ContextTree); !ok || got != ActionEdit {
				t.Fatalf("configured editor action = (%q, %t), want note edit", got, ok)
			}
			return
		}
	}
	t.Fatal("ActionRegistry() has no note.edit action")
}

func TestModelEditsSelectedNoteAndReloadsAfterEveryExit(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name        string
		exit        string
		wantWarning bool
	}{
		{name: "success", exit: "0"},
		{name: "failure", exit: "1", wantWarning: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := loadedModel(t)
			model, render := updateModelCommand(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
			model = updateModel(t, model, run(render))
			model.editorCommand = editor.Command{
				Executable: executable,
				Args:       []string{"-test.run=^TestFakeUIEditorProcess$", "--", tt.exit},
			}
			harness := &editHarness{Model: model}
			result, err := tea.NewProgram(harness, tea.WithInput(nil), tea.WithOutput(io.Discard)).Run()
			if err != nil {
				t.Fatal(err)
			}
			got := result.(*editHarness).Model
			if string(got.document.Content) != "# Edited\n" {
				t.Fatalf("reloaded document = %q, want external edit", got.document.Content)
			}
			if hasWarning := strings.Contains(got.status, "editor:"); hasWarning != tt.wantWarning {
				t.Errorf("status = %q, wantWarning %t", got.status, tt.wantWarning)
			}
		})
	}
}

func TestModelDoesNotEditDirectory(t *testing.T) {
	model := loadedModel(t)
	model = updateModel(t, model, key("k"))
	model = updateModel(t, model, key("k"))
	selected, ok := model.tree.Selected()
	if !ok || selected.Kind != notes.KindDirectory {
		t.Fatalf("selected = %#v, want directory", selected)
	}
	_, command := updateModelCommand(t, model, key("e"))
	if command != nil {
		t.Fatal("editing a directory returned a command")
	}
}

type editHarness struct {
	Model
	edited bool
}

func (m *editHarness) Init() tea.Cmd { return m.editSelected() }

func (m *editHarness) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	next, command := m.Model.Update(message)
	m.Model = next.(Model)
	switch message.(type) {
	case editorFinished:
		m.edited = true
	case readResult:
		if m.edited {
			return m, tea.Quit
		}
	}
	return m, command
}

func TestFakeUIEditorProcess(t *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 {
		return
	}
	arguments := os.Args[separator+1:]
	if len(arguments) != 2 || !filepath.IsAbs(arguments[1]) {
		os.Exit(2)
	}
	if err := os.WriteFile(arguments[1], []byte("# Edited\n"), 0o600); err != nil {
		os.Exit(2)
	}
	if arguments[0] == "1" {
		os.Exit(1)
	}
}
