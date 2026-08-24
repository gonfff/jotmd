package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/notes"
)

func TestTinyTerminalKeepsDestructiveConfirmationVisible(t *testing.T) {
	for _, test := range []struct {
		name string
		mode Mode
		want string
	}{
		{name: "trash", mode: TrashConfirm, want: "Move note.md to Trash?"},
		{name: "delete", mode: PermanentDeleteConfirm, want: "Permanently delete note.md"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := testModel(t)
			model.width, model.height = tinyWidth-1, tinyHeight-1
			model.mode = test.mode
			model.modalEntry = notes.Entry{Name: "note.md"}
			if view := model.View().Content; !strings.Contains(view, test.want) {
				t.Fatalf("tiny destructive modal = %q, want %q", view, test.want)
			}
		})
	}
}

func TestDestructiveConfirmationsUseStatusAndExplicitKeys(t *testing.T) {
	for _, test := range []struct {
		name string
		key  string
		mode Mode
	}{
		{name: "trash", key: "d", mode: TrashConfirm},
		{name: "delete", key: "D", mode: PermanentDeleteConfirm},
	} {
		t.Run(test.name, func(t *testing.T) {
			open := func(t *testing.T) Model {
				t.Helper()
				model := sizedLoadedModel(t)
				model.cfg.StatusBar = false
				return updateModel(t, model, key(test.key))
			}

			model := open(t)
			if popup := model.popupContent(78, 10); popup != "" {
				t.Fatalf("popupContent() = %q, want status confirmation", popup)
			}
			status := ansi.Strip(model.statusView(model.width, model.focus))
			if !model.statusBarVisible() || !strings.Contains(status, "note.md") || !strings.Contains(status, "y/Enter confirm  Esc cancel") {
				t.Fatalf("status = %q, visible=%t", status, model.statusBarVisible())
			}

			for _, confirmation := range []string{"y", "enter"} {
				model, command := updateModelCommand(t, open(t), key(confirmation))
				if command == nil || model.mode != Browse {
					t.Fatalf("%q = (command=%t, mode=%v), want mutation command and Browse", confirmation, command != nil, model.mode)
				}
			}

			model, command := updateModelCommand(t, open(t), key("esc"))
			if command != nil || model.mode != Browse {
				t.Fatalf("esc = (command=%t, mode=%v), want no command and Browse", command != nil, model.mode)
			}

			for _, ignored := range []string{"n", "q"} {
				model, command := updateModelCommand(t, open(t), key(ignored))
				if command != nil || model.mode != test.mode {
					t.Fatalf("%q = (command=%t, mode=%v), want no command and %v", ignored, command != nil, model.mode, test.mode)
				}
			}
		})
	}
}

func TestDestructiveConfirmationsSyncTreeViewportWhenTheyForceStatusBar(t *testing.T) {
	for _, test := range []struct {
		name  string
		open  string
		close string
	}{
		{name: "trash/y", open: "d", close: "y"},
		{name: "trash/enter", open: "d", close: "enter"},
		{name: "trash/esc", open: "d", close: "esc"},
		{name: "delete/y", open: "D", close: "y"},
		{name: "delete/enter", open: "D", close: "enter"},
		{name: "delete/esc", open: "D", close: "esc"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.StatusBar = false
			model := newModel(testStore(t), cfg, jotmdTheme(t))
			model = updateModel(t, model, run(model.Init()))
			model = updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})

			model = updateModel(t, model, key(test.open))
			if got, want := model.tree.height, paneHeight(model.height, true); got != want {
				t.Errorf("open tree height = %d, want %d", got, want)
			}

			model = updateModel(t, model, key(test.close))
			if got, want := model.tree.height, paneHeight(model.height, false); got != want {
				t.Errorf("close tree height = %d, want %d", got, want)
			}
		})
	}
}

func TestLayoutIndependentTrashConfirmationUsesPhysicalY(t *testing.T) {
	model := updateModel(t, sizedLoadedModel(t), key("d"))
	_, command := updateModelCommand(t, model, tea.KeyPressMsg(tea.Key{Code: 'н', BaseCode: 'y', Text: "н"}))
	if command == nil {
		t.Fatal("physical y did not confirm trash")
	}
}

func TestTrashConflictRescansWithoutPermanentDeleteFallback(t *testing.T) {
	model := sizedLoadedModel(t)
	root := model.storeRootForTest(t)
	model = updateModel(t, model, key("d"))
	note := filepath.Join(root, "note.md")
	if err := os.Remove(note); err != nil {
		t.Fatal(err)
	}
	writeNote(t, note, "replacement\n")

	model, command := updateModelCommand(t, model, key("y"))
	model, scan := updateModelCommand(t, model, run(command))
	if scan == nil || !model.loading {
		t.Fatal("trash conflict did not start a full rescan")
	}
	model = updateModel(t, model, run(scan))
	content, err := os.ReadFile(note)
	if err != nil || string(content) != "replacement\n" {
		t.Fatalf("trash conflict permanently deleted replacement: content=%q err=%v", content, err)
	}
	if !strings.Contains(model.status, "filesystem entry changed") {
		t.Fatalf("trash conflict status = %q", model.status)
	}
}

func TestTrashPermissionDeniedShowsAutomationInstructionsInStatusBar(t *testing.T) {
	model := updateModel(t, sizedLoadedModel(t), mutationResult{err: notes.ErrTrashPermissionDenied})
	status := ansi.Strip(model.statusView(model.width, model.focus))
	for _, want := range []string{"Finder", "Privacy & Security", "Automation"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status = %q, want %q", status, want)
		}
	}
}

func TestPermanentDeleteDeletesOnY(t *testing.T) {
	model := sizedLoadedModel(t)
	root := model.storeRootForTest(t)
	model = updateModel(t, model, key("D"))
	model, command := updateModelCommand(t, model, key("y"))
	model = finishMutation(t, model, command)
	if _, err := os.Stat(filepath.Join(root, "note.md")); !os.IsNotExist(err) {
		t.Fatalf("deleted note error = %v, want not exist", err)
	}
}

func TestPermanentDeleteDeletesDirectoriesOnY(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("k"))
	model = updateModel(t, model, key("k"))
	model = updateModel(t, model, key("D"))
	model, command := updateModelCommand(t, model, key("y"))
	model = finishMutation(t, model, command)
	if _, err := os.Stat(filepath.Join(model.storeRootForTest(t), "docs")); !os.IsNotExist(err) {
		t.Fatalf("deleted directory error = %v, want not exist", err)
	}
}

func TestPermanentDeleteUsesSnapshotIdentity(t *testing.T) {
	model := sizedLoadedModel(t)
	root := model.storeRootForTest(t)
	model = updateModel(t, model, key("D"))
	note := filepath.Join(root, "note.md")
	if err := os.Remove(note); err != nil {
		t.Fatal(err)
	}
	writeNote(t, note, "replacement\n")
	model, command := updateModelCommand(t, model, key("y"))
	model, scan := updateModelCommand(t, model, run(command))
	model = updateModel(t, model, run(scan))
	if content, err := os.ReadFile(note); err != nil || string(content) != "replacement\n" {
		t.Fatalf("identity conflict deleted replacement: content=%q err=%v", content, err)
	}
	if !strings.Contains(model.status, notes.ErrConflict.Error()) {
		t.Fatalf("delete conflict status = %q", model.status)
	}
}

func TestPermanentDeleteSelectsParentAfterRescan(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "a", "keep.md"), "keep\n")
	writeNote(t, filepath.Join(root, "z", "note.md"), "delete\n")
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	model := newModel(store, config.Defaults(), jotmdTheme(t))
	model = updateModel(t, model, run(model.Init()))
	model.tree, _ = model.tree.Select("z/note.md")
	model = updateModel(t, model, key("D"))
	model, command := updateModelCommand(t, model, key("y"))
	model = finishMutation(t, model, command)
	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "z" {
		t.Fatalf("selection after delete = %q, want z", selected.Path)
	}
}

func TestPermanentDeleteCurrentNoteExitsFullPreview(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("enter"))
	model = updateModel(t, model, key("D"))
	model, command := updateModelCommand(t, model, key("y"))

	model = finishMutation(t, model, command)

	if model.fullPreview || model.focus != ContextTree || model.hasDocument {
		t.Fatalf("post-delete layout = (full=%t, focus=%q, document=%t), want tree layout without document", model.fullPreview, model.focus, model.hasDocument)
	}
}
