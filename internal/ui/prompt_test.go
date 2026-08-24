package ui

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/notes"
)

func TestNewNotePromptOwnsPrintableKeysAndEscapeCancels(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("n"))
	model, command := updateModelCommand(t, model, key("q"))
	if command != nil {
		if _, quits := run(command).(tea.QuitMsg); quits {
			t.Fatal("q quit while the new-note prompt was open")
		}
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "New note") || !strings.Contains(view, "q") {
		t.Fatalf("new-note prompt = %q, want typed q", view)
	}

	model = updateModel(t, model, key("esc"))
	if view := ansi.Strip(model.View().Content); strings.Contains(view, "New note") {
		t.Fatalf("escape left prompt open: %q", view)
	}
}

func TestNewNotePromptRejectsEmptyTitle(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("n"))
	model, command := updateModelCommand(t, model, key("enter"))
	if command != nil {
		t.Fatal("empty title started a mutation")
	}
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "New note") || !strings.Contains(view, "required") {
		t.Fatalf("empty-title prompt = %q, want validation error", view)
	}
}

func TestTextPromptsRenderInColoredStatusBarWithoutPopup(t *testing.T) {
	for _, test := range []struct {
		name, key, title string
	}{
		{name: "new note", key: "n", title: "New note"},
		{name: "new directory", key: "N", title: "New directory"},
		{name: "rename", key: "r", title: "Rename note.md"},
		{name: "copy", key: "c", title: "Copy note.md"},
		{name: "move", key: "m", title: "Move note.md"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := sizedLoadedModel(t)
			model.cfg.StatusBar = false
			model = updateModel(t, model, key(test.key))
			if popup := model.popupContent(78, 10); popup != "" {
				t.Fatalf("popupContent() = %q, want prompt only in status bar", popup)
			}
			if !model.statusBarVisible() {
				t.Fatal("status bar is hidden while prompt is active")
			}
			status := model.statusView(model.width, model.focus)
			if stripped := ansi.Strip(status); ansi.StringWidth(stripped) != model.width || !strings.Contains(stripped, test.title) {
				t.Fatalf("statusView() = %q, want full-width %q prompt", stripped, test.title)
			}
			backgroundPrefix := strings.SplitN(lipgloss.NewStyle().
				Foreground(lipgloss.Color(model.theme.Palette.SelectionForeground)).
				Background(lipgloss.Color(model.theme.Palette.Status)).
				Render("x"), "x", 2)[0]
			if backgroundPrefix != "" && !strings.Contains(status, backgroundPrefix) {
				t.Fatalf("statusView() has no status background: %q", status)
			}
		})
	}
}

func TestCopyAndMoveStartEmptyAndCompleteVaultRelativeDestination(t *testing.T) {
	model := sizedLoadedModel(t)
	root := model.storeRootForTest(t)
	for _, keyValue := range []string{"c", "m"} {
		model = updateModel(t, model, key(keyValue))
		if model.input.Value() != "" || !model.input.ShowSuggestions {
			t.Fatalf("%q prompt = (%q, suggestions=%t), want empty input with suggestions", keyValue, model.input.Value(), model.input.ShowSuggestions)
		}
		suggestions := model.input.AvailableSuggestions()
		if !slices.Contains(suggestions, "note.md") || !slices.Contains(suggestions, "docs/note.md") || slices.Contains(suggestions, "note.md/note.md") {
			t.Fatalf("%q suggestions = %q", keyValue, suggestions)
		}
		model = updateModel(t, model, key("d"))
		if status := ansi.Strip(model.statusView(model.width, model.focus)); !strings.Contains(status, model.input.CurrentSuggestion()) {
			t.Fatalf("%q status = %q, want current suggestion %q", keyValue, status, model.input.CurrentSuggestion())
		}
		model = updateModel(t, model, key("esc"))
	}

	model = updateModel(t, model, key("c"))
	model = updateModel(t, model, key("d"))
	model = updateModel(t, model, key("tab"))
	if model.input.Value() != "docs/" {
		t.Fatalf("completed copy directory = %q, want docs/", model.input.Value())
	}
	model = updateModel(t, model, key("tab"))
	if model.input.Value() != "docs/note.md" {
		t.Fatalf("completed copy destination = %q, want docs/note.md", model.input.Value())
	}
	model, command := updateModelCommand(t, model, key("enter"))
	model = finishMutation(t, model, command)
	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "docs/note.md" {
		t.Fatalf("copied selection = %q, want docs/note.md", selected.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "note.md")); err != nil {
		t.Fatalf("copy removed source: %v", err)
	}

	model.tree, _ = model.tree.Select("note.md")
	model = updateModel(t, model, key("m"))
	model = updateModel(t, model, ctrlKey('u'))
	model = updateModel(t, model, key("moved.md"))
	model, command = updateModelCommand(t, model, key("enter"))
	model = finishMutation(t, model, command)
	selected, ok = model.tree.Selected()
	if !ok || selected.Path != "moved.md" {
		t.Fatalf("moved selection = %q, want moved.md", selected.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "note.md")); !os.IsNotExist(err) {
		t.Fatalf("move source stat error = %v, want not exist", err)
	}
}

func TestCopyAndMoveAltBackspaceDeletesPathSegment(t *testing.T) {
	for _, keyValue := range []string{"c", "m"} {
		t.Run(keyValue, func(t *testing.T) {
			model := sizedLoadedModel(t)
			model = updateModel(t, model, key(keyValue))

			model.input.SetValue("docs/note.md/keep")
			model.input.SetCursor(len([]rune("docs/note.md")))
			model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace, Mod: tea.ModAlt}))
			if got := model.input.Value(); got != "docs/keep" {
				t.Fatalf("path segment deletion = %q, want docs/keep", got)
			}

			model.input.SetValue("note.md/keep")
			model.input.SetCursor(len([]rune("note.md")))
			model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace, Mod: tea.ModAlt}))
			if got := model.input.Value(); got != "/keep" {
				t.Fatalf("root path segment deletion = %q, want /keep", got)
			}

			model.input.SetValue("docs/note.md")
			model.input.CursorEnd()
			model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace, Mod: tea.ModAlt}))
			if got := model.input.Value(); got != "docs" {
				t.Fatalf("end-of-path deletion = %q, want docs", got)
			}

			model.input.SetValue("note.md")
			model.input.CursorEnd()
			model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace, Mod: tea.ModAlt}))
			if got := model.input.Value(); got != "" {
				t.Fatalf("whole input deletion = %q, want empty", got)
			}
		})
	}
}

func TestCopyAndMovePathCompletionStopsAtUnambiguousBoundaries(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("c"))
	suggestions := []string{
		"superpowers/plans/note.md",
		"superpowers/specs/note.md",
	}

	model.input.SetValue("s")
	model.input.SetSuggestions(suggestions)
	model = updateModel(t, model, key("tab"))
	if got := model.input.Value(); got != "superpowers/" {
		t.Fatalf("first completion = %q, want superpowers/", got)
	}

	model = updateModel(t, model, key("tab"))
	if got := model.input.Value(); got != "superpowers/" {
		t.Fatalf("ambiguous completion = %q, want unchanged superpowers/", got)
	}

	model.input.SetValue("superpowers/p")
	model.input.SetSuggestions(suggestions)
	model = updateModel(t, model, key("tab"))
	if got := model.input.Value(); got != "superpowers/plans/" {
		t.Fatalf("directory completion = %q, want superpowers/plans/", got)
	}

	model = updateModel(t, model, key("tab"))
	if got := model.input.Value(); got != "superpowers/plans/note.md" {
		t.Fatalf("filename completion = %q, want superpowers/plans/note.md", got)
	}

	unicodeSuggestions := []string{
		"заметки/планы/note.md",
		"заметки/спеки/note.md",
	}
	model.input.SetValue("з")
	model.input.SetSuggestions(unicodeSuggestions)
	model = updateModel(t, model, key("tab"))
	if got := model.input.Value(); got != "заметки/" {
		t.Fatalf("Unicode completion = %q, want заметки/", got)
	}
}

func TestCopyAndMoveSuggestionUsesMutedForegroundAndHidesExactInput(t *testing.T) {
	for _, keyValue := range []string{"c", "m"} {
		t.Run(keyValue, func(t *testing.T) {
			model := sizedLoadedModel(t)
			model.theme.Palette.SelectionForeground = "#010203"
			model.theme.Palette.Muted = "#A0A0A0"
			model.theme.Palette.Status = "#040506"
			model = updateModel(t, model, key(keyValue))
			model.input.SetValue("note.md")

			status := model.statusView(model.width, model.focus)
			if count := strings.Count(ansi.Strip(status), "note.md"); count != 2 {
				t.Fatalf("exact-input suggestion occurrences = %d, want title and input only: %q", count, ansi.Strip(status))
			}

			model = updateModel(t, model, ctrlKey('u'))
			model = updateModel(t, model, key("d"))
			status = model.statusView(model.width, model.focus)
			want := lipgloss.NewStyle().
				Foreground(lipgloss.Color(model.theme.Palette.Muted)).
				Background(lipgloss.Color(model.theme.Palette.Status)).
				Render("docs/note.md")
			if !strings.Contains(status, want) {
				t.Fatalf("suggestion has no muted status style: %q", status)
			}
		})
	}
}

func TestCopyAndMoveAcceptSelectedDirectory(t *testing.T) {
	for _, test := range []struct {
		key  string
		mode Mode
	}{
		{key: "c", mode: CopyPrompt},
		{key: "m", mode: MovePrompt},
	} {
		model := sizedLoadedModel(t)
		model.tree, _ = model.tree.Select("docs")
		model = updateModel(t, model, key(test.key))
		if model.mode != test.mode || model.modalEntry.Path != "docs" {
			t.Errorf("key %q on directory = (%v, %q), want (%v, docs)", test.key, model.mode, model.modalEntry.Path, test.mode)
		}
		for _, suggestion := range model.input.AvailableSuggestions() {
			if suggestion == "docs" || strings.HasPrefix(suggestion, "docs/") {
				t.Errorf("key %q suggested destination inside source: %q", test.key, suggestion)
			}
		}
	}
}

func TestStatusPromptKeepsOneBackgroundAcrossCursor(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("r"))
	model.input.SetCursor(4)
	status := model.statusView(model.width, model.focus)
	if !strings.Contains(ansi.Strip(status), "note▏.md") {
		t.Fatalf("status cursor = %q, want cursor at input position", ansi.Strip(status))
	}
	if resets := strings.Count(status, "\x1b[m"); resets != 1 {
		t.Fatalf("status ANSI resets = %d, want one continuous background: %q", resets, status)
	}
}

func TestLongStatusPromptKeepsCursorVisible(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("n"))
	model.input.SetValue(strings.Repeat("long-name-", 10))
	model.input.CursorEnd()
	status := ansi.Strip(model.statusView(24, model.focus))
	if ansi.StringWidth(status) != 24 || !strings.Contains(status, "▏") || !strings.Contains(status, "name-") {
		t.Fatalf("long status prompt = %q, want visible tail and cursor", status)
	}
}

func TestCreateNoteUsesSelectedDirectoryAndSelectsCreatedNote(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("k"))
	model = updateModel(t, model, key("n"))
	model = updateModel(t, model, key("Inside"))
	model, command := updateModelCommand(t, model, key("enter"))
	model = finishMutation(t, model, command)

	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "docs/inside.md" || !model.hasDocument || model.document.Path != selected.Path {
		t.Fatalf("created selection/document = (%q, %q), want docs/inside.md", selected.Path, model.document.Path)
	}
	if _, err := os.Stat(filepath.Join(model.storeRootForTest(t), "docs", "inside.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRootNoteWhenOnlyEntryIsDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "existing.md"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	model := newModel(store, config.Defaults(), jotmdTheme(t))
	model.cfg.Watch = false
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 16})
	model = updateModel(t, model, run(model.Init()))
	model = updateModel(t, model, key("n"))
	model = updateModel(t, model, key("root"))
	model, command := updateModelCommand(t, model, key("enter"))
	model = finishMutation(t, model, command)

	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "root.md" {
		t.Fatalf("selected = %q, want root.md", selected.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "root.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCreateDirectoryUsesParentOfSelectedNoteAndSelectsResult(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("N"))
	model = updateModel(t, model, key("work"))
	model, command := updateModelCommand(t, model, key("enter"))
	model = finishMutation(t, model, command)

	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "work" || selected.Kind != notes.KindDirectory {
		t.Fatalf("created directory selection = %#v, want work", selected)
	}
}

func TestCreateFromEmptyVaultUsesRoot(t *testing.T) {
	for _, test := range []struct {
		name string
		key  string
		want notes.RelPath
	}{
		{name: "note", key: "n", want: "first.md"},
		{name: "directory", key: "N", want: "first"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store, err := notes.NewStore(root, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			model := newModel(store, config.Defaults(), jotmdTheme(t))
			model = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 16})
			model = updateModel(t, model, run(model.Init()))
			model = updateModel(t, model, key(test.key))
			model = updateModel(t, model, key("first"))
			model, command := updateModelCommand(t, model, key("enter"))
			model = finishMutation(t, model, command)
			selected, ok := model.tree.Selected()
			if !ok || selected.Path != test.want {
				t.Fatalf("selected = %q, want %q", selected.Path, test.want)
			}
		})
	}
}

func TestRenameUsesSnapshotIdentityAndRescansAfterConflict(t *testing.T) {
	model := sizedLoadedModel(t)
	root := model.storeRootForTest(t)
	model = updateModel(t, model, key("r"))

	note := filepath.Join(root, "note.md")
	if err := os.Remove(note); err != nil {
		t.Fatal(err)
	}
	writeNote(t, note, "replacement\n")
	model = updateModel(t, model, ctrlKey('u'))
	model = updateModel(t, model, key("renamed.md"))
	model, command := updateModelCommand(t, model, key("enter"))
	if command == nil {
		t.Fatal("rename did not start")
	}
	model = updateModel(t, model, run(command))
	if !model.loading {
		t.Fatal("rename conflict did not start a full rescan")
	}
	model = updateModel(t, model, run(model.scanCommand(model.scanCtx, model.scanGeneration)))
	if !strings.Contains(model.status, "filesystem entry changed") {
		t.Fatalf("rename conflict status = %q", model.status)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed.md")); !os.IsNotExist(err) {
		t.Fatalf("renamed path error = %v, want not exist", err)
	}
}

func TestMutationWarningDoesNotMaskLaterEditorFailure(t *testing.T) {
	model := sizedLoadedModel(t)
	model, scan := updateModelCommand(t, model, mutationResult{err: errors.New("rename failed")})
	model, read := updateModelCommand(t, model, run(scan))
	model = updateModel(t, model, run(read))
	if !strings.Contains(model.status, "rename failed") {
		t.Fatalf("mutation status after reload = %q", model.status)
	}

	model, scan = updateModelCommand(t, model, editorFinished{err: errors.New("editor failed")})
	model, read = updateModelCommand(t, model, run(scan))
	model = updateModel(t, model, run(read))
	if !strings.Contains(model.status, "editor failed") || strings.Contains(model.status, "rename failed") {
		t.Fatalf("status after editor reload = %q, want newer editor failure", model.status)
	}
}

func TestMutationBindingsDriveDispatchFooterAndHelp(t *testing.T) {
	model := NewModelWithOptions(testStore(t), config.Defaults(), jotmdTheme(t), config.Keymap{Bindings: []config.Binding{
		{Action: "note.new", Keys: []string{"c"}},
	}}, nil)
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 16})
	model = updateModel(t, model, run(model.Init()))
	if footer := ansi.Strip(model.statusView(120, ContextTree)); ansi.StringWidth(footer) != 120 || !strings.HasSuffix(footer, "? help  ") {
		t.Fatalf("footer = %q, want inset ? help", footer)
	}
	model = updateModel(t, model, key("?"))
	if help := ansi.Strip(model.View().Content); !strings.Contains(help, "c  new note") {
		t.Fatalf("help = %q, want effective new-note binding", help)
	}
}

func sizedLoadedModel(t *testing.T) Model {
	t.Helper()
	model := loadedModel(t)
	return updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 16})
}

func finishMutation(t *testing.T, model Model, command tea.Cmd) Model {
	t.Helper()
	if command == nil {
		t.Fatal("mutation did not return a command")
	}
	model, scan := updateModelCommand(t, model, run(command))
	if scan == nil || !model.loading {
		t.Fatal("mutation did not start a full rescan")
	}
	model, read := updateModelCommand(t, model, run(scan))
	if read != nil {
		model = updateModel(t, model, run(read))
	}
	return model
}

func ctrlKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: tea.ModCtrl})
}

func (m Model) storeRootForTest(t *testing.T) string {
	t.Helper()
	absolute, err := m.store.AbsolutePath("note.md")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(absolute)
}
