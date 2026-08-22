package ui

import (
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/notes"
	"github.com/gonfff/jotmd/internal/theme"
)

func TestTreeReconcileRecoversUnambiguousRenameByIdentity(t *testing.T) {
	identity := notes.FileIdentity{Device: 1, Inode: 2}
	tree, ok := NewTree(notes.Snapshot{Entries: []notes.Entry{
		{Path: "a.md", Name: "a.md", Kind: notes.KindMarkdown},
		{Path: "b.md", Name: "b.md", Kind: notes.KindMarkdown, Identity: identity},
	}}).Select("b.md")
	if !ok {
		t.Fatal("Select() did not find b.md")
	}

	tree = tree.Reconcile(notes.Snapshot{Entries: []notes.Entry{
		{Path: "a.md", Name: "a.md", Kind: notes.KindMarkdown},
		{Path: "renamed.md", Name: "renamed.md", Kind: notes.KindMarkdown, Identity: identity},
	}})

	assertSelectedPath(t, tree, "renamed.md")
}

func TestTreeReconcileDeletedSelectionFallsBackToPreviousSiblingThenParent(t *testing.T) {
	old := notes.Snapshot{Entries: []notes.Entry{
		entry("docs", notes.KindDirectory),
		entry("docs/a.md", notes.KindMarkdown),
		entry("docs/b.md", notes.KindMarkdown),
	}}
	tree, _ := NewTree(old).Select("docs/b.md")
	tree = tree.Reconcile(notes.Snapshot{Entries: []notes.Entry{
		entry("docs", notes.KindDirectory),
		entry("docs/a.md", notes.KindMarkdown),
	}})
	assertSelectedPath(t, tree, "docs/a.md")

	tree, _ = NewTree(old).Select("docs/a.md")
	tree = tree.Reconcile(notes.Snapshot{Entries: []notes.Entry{entry("docs", notes.KindDirectory)}})
	assertSelectedPath(t, tree, "docs")
}

func TestTreeReconcileExpandsNewDirectoriesAndRetainsCollapsedOnes(t *testing.T) {
	tree := NewTree(notes.Snapshot{Entries: []notes.Entry{
		entry("docs", notes.KindDirectory),
		entry("docs/old.md", notes.KindMarkdown),
	}}).Update(ActionCollapse)

	tree = tree.Reconcile(notes.Snapshot{Entries: []notes.Entry{
		entry("docs", notes.KindDirectory),
		entry("docs/old.md", notes.KindMarkdown),
		entry("new", notes.KindDirectory),
		entry("new/note.md", notes.KindMarkdown),
	}})
	if tree.expanded["docs"] {
		t.Fatal("explicitly collapsed docs directory was expanded")
	}
	if !tree.expanded["new"] {
		t.Fatal("new directory was not expanded")
	}
	visible := tree.visible()
	for _, entry := range visible {
		if entry.Path == "docs/old.md" {
			t.Fatal("descendant of collapsed docs directory is visible")
		}
	}
	for _, path := range []notes.RelPath{"docs", "new", "new/note.md"} {
		found := false
		for _, entry := range visible {
			if entry.Path == path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("visible entries do not contain %q", path)
		}
	}
}

func TestTreeReconcileAmbiguousIdentityDoesNotGuessRename(t *testing.T) {
	identity := notes.FileIdentity{Device: 1, Inode: 2}
	tree, _ := NewTree(notes.Snapshot{Entries: []notes.Entry{
		{Path: "old.md", Name: "old.md", Kind: notes.KindMarkdown, Identity: identity},
	}}).Select("old.md")
	tree = tree.Reconcile(notes.Snapshot{Entries: []notes.Entry{
		{Path: "one.md", Name: "one.md", Kind: notes.KindMarkdown, Identity: identity},
		{Path: "two.md", Name: "two.md", Kind: notes.KindMarkdown, Identity: identity},
	}})
	if selected, ok := tree.Selected(); ok {
		t.Fatalf("ambiguous rename selected %q, want no guessed selection", selected.Path)
	}
}

func TestModelContentReloadPreservesApproximatePreviewScrollRatio(t *testing.T) {
	model := loadedModel(t)
	model.width, model.height = 80, 12
	model.preview.render = func(doc notes.Document, _ int, _ bool, _ theme.Theme, _ string) (string, error) {
		return string(doc.Content), nil
	}
	old := notes.Document{Path: "note.md", Revision: "old", Content: []byte(strings.TrimSuffix(strings.Repeat("line\n", 20), "\n"))}
	if err := model.preview.SetDocument(old, 30, paneHeight(model.height, model.statusBarVisible()), jotmdTheme(t)); err != nil {
		t.Fatal(err)
	}
	model.document, model.hasDocument = old, true
	model.preview.viewport.SetYOffset(4)
	wantRatio := model.preview.viewport.ScrollPercent()
	selected, _ := model.tree.Selected()
	model = updateModel(t, model, scanResult{generation: model.scanGeneration, snapshot: model.tree.snapshot})
	updated := notes.Document{Path: selected.Path, Revision: "new", Content: []byte(strings.TrimSuffix(strings.Repeat("line\n", 28), "\n"))}
	model, render := updateModelCommand(t, model, readResult{generation: model.readGeneration, path: selected.Path, document: updated})
	model = updateModel(t, model, run(render))
	if got := model.preview.viewport.ScrollPercent(); math.Abs(got-wantRatio) > 0.15 {
		t.Fatalf("scroll ratio = %.2f, want approximately %.2f", got, wantRatio)
	}
}

func TestModelSelectionFallbackDoesNotReuseDeletedDocumentScroll(t *testing.T) {
	model := loadedModel(t)
	model.width, model.height = 80, 12
	model.preview.render = func(doc notes.Document, _ int, _ bool, _ theme.Theme, _ string) (string, error) {
		return string(doc.Content), nil
	}
	first := notes.Entry{Path: "a.md", Name: "a.md", Kind: notes.KindMarkdown, Identity: notes.FileIdentity{Device: 1, Inode: 1}}
	second := notes.Entry{Path: "b.md", Name: "b.md", Kind: notes.KindMarkdown, Identity: notes.FileIdentity{Device: 1, Inode: 2}}
	model.tree = NewTree(notes.Snapshot{Entries: []notes.Entry{first, second}})
	model.document = notes.Document{Path: first.Path, Revision: "first", Content: []byte("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12")}
	model.hasDocument = true
	model.preview.viewport.SetWidth(30)
	model.preview.viewport.SetHeight(2)
	model.preview.viewport.SetContent("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12")
	model.preview.viewport.SetYOffset(8)
	model = updateModel(t, model, scanResult{generation: model.scanGeneration, snapshot: notes.Snapshot{Entries: []notes.Entry{second}}})
	replacement, ok := model.tree.Selected()
	if !ok || replacement.Path != second.Path {
		t.Fatal("scan did not select a replacement")
	}
	doc := notes.Document{Path: replacement.Path, Revision: "replacement", Content: []byte("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl")}
	model, render := updateModelCommand(t, model, readResult{generation: model.readGeneration, path: replacement.Path, document: doc})
	model = updateModel(t, model, run(render))
	if got := model.preview.viewport.YOffset(); got != 0 {
		t.Fatalf("replacement document offset = %d, want top", got)
	}
}

func TestModelPendingSelectionDoesNotReusePreviousDocumentScroll(t *testing.T) {
	model := loadedModel(t)
	model.width, model.height = 80, 12
	model.preview.render = func(doc notes.Document, _ int, _ bool, _ theme.Theme, _ string) (string, error) {
		return string(doc.Content), nil
	}
	first := notes.Entry{Path: "a.md", Name: "a.md", Kind: notes.KindMarkdown, Identity: notes.FileIdentity{Device: 1, Inode: 1}}
	second := notes.Entry{Path: "b.md", Name: "b.md", Kind: notes.KindMarkdown, Identity: notes.FileIdentity{Device: 1, Inode: 2}}
	model.tree = NewTree(notes.Snapshot{Entries: []notes.Entry{first, second}})
	model.document = notes.Document{Path: first.Path, Revision: "first", Content: []byte("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12")}
	model.hasDocument = true
	model.preview.viewport.SetWidth(30)
	model.preview.viewport.SetHeight(2)
	model.preview.viewport.SetContent("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12")
	model.preview.viewport.SetYOffset(8)
	model.pendingSelect = second.Path
	model = updateModel(t, model, scanResult{generation: model.scanGeneration, snapshot: model.tree.snapshot})
	doc := notes.Document{Path: second.Path, Revision: "second", Content: []byte("a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl")}
	model, render := updateModelCommand(t, model, readResult{generation: model.readGeneration, path: second.Path, document: doc})
	model = updateModel(t, model, run(render))
	if got := model.preview.viewport.YOffset(); got != 0 {
		t.Fatalf("newly selected document offset = %d, want top", got)
	}
}

func TestModelFocusAndReloadAlwaysStartScanAndViewReportsFocus(t *testing.T) {
	model := loadedModel(t)
	before := model.scanGeneration
	model, focusScan := updateModelCommand(t, model, tea.FocusMsg{})
	if focusScan == nil || model.scanGeneration != before+1 {
		t.Fatal("FocusMsg did not start a scan")
	}
	model, reloadScan := updateModelCommand(t, model, key("R"))
	if reloadScan == nil || model.scanGeneration != before+2 {
		t.Fatal("R did not start a scan")
	}
	if !model.View().ReportFocus {
		t.Fatal("View().ReportFocus = false, want true")
	}
}

func TestModelWatcherFailurePersistsWithoutDisablingReload(t *testing.T) {
	model := loadedModel(t)
	model = updateModel(t, model, watchStarted{generation: model.watchGeneration, err: assertError("boom")})
	model.status = ""
	if footer := model.statusView(120, ContextTree); !containsAll(footer, "watch: boom") {
		t.Fatalf("status = %q, want persistent watcher warning", footer)
	}
	_, command := updateModelCommand(t, model, key("R"))
	if command == nil {
		t.Fatal("R was disabled after watcher failure")
	}
}

func TestModelWatchFalseDoesNotStartWatcher(t *testing.T) {
	cfg := config.Defaults()
	cfg.Watch = false
	model := newModel(testStore(t), cfg, jotmdTheme(t))
	model = updateModel(t, model, run(model.Init()))
	if model.watchGeneration != 0 {
		t.Fatalf("watch generation = %d, want disabled", model.watchGeneration)
	}
}

func TestModelWatchChangeAlwaysStartsFullScan(t *testing.T) {
	model := loadedModel(t)
	model.watchGeneration = 1
	before := model.scanGeneration
	changes := make(chan notes.Change)
	next, command := updateModelCommand(t, model, watchResult{
		generation: model.watchGeneration,
		changes:    changes,
		change:     notes.Change{Paths: []notes.RelPath{"note.md"}},
		open:       true,
	})
	if command == nil || next.scanGeneration != before+1 {
		t.Fatal("watch change did not start a full scan")
	}
}

func TestModelUnrelatedScanDoesNotReadUnchangedSelection(t *testing.T) {
	model := loadedModel(t)
	before := model.readGeneration
	snapshot := model.tree.snapshot
	snapshot.Entries = append([]notes.Entry(nil), snapshot.Entries...)
	snapshot.Entries = append(snapshot.Entries, notes.Entry{Path: "other.md", Name: "other.md", Kind: notes.KindMarkdown})
	next, command := updateModelCommand(t, model, scanResult{generation: model.scanGeneration, snapshot: snapshot})
	if command != nil || next.readGeneration != before {
		t.Fatalf("unchanged selection scheduled read: command=%T generation=%d, want nil/%d", command, next.readGeneration, before)
	}
}

func TestModelSelectedEntryChangeSchedulesRead(t *testing.T) {
	model := loadedModel(t)
	before := model.readGeneration
	snapshot := model.tree.snapshot
	snapshot.Entries = append([]notes.Entry(nil), snapshot.Entries...)
	for index := range snapshot.Entries {
		if snapshot.Entries[index].Path == model.document.Path {
			snapshot.Entries[index].Size++
		}
	}
	next, command := updateModelCommand(t, model, scanResult{generation: model.scanGeneration, snapshot: snapshot})
	if command == nil || next.readGeneration != before+1 {
		t.Fatalf("changed selection read = command %T/generation %d, want command/%d", command, next.readGeneration, before+1)
	}
}

func TestModelEditorReturnForcesOneReadWithUnchangedSnapshot(t *testing.T) {
	model := loadedModel(t)
	snapshot := model.tree.snapshot
	before := model.readGeneration
	model, _ = updateModelCommand(t, model, editorFinished{})

	model, read := updateModelCommand(t, model, scanResult{generation: model.scanGeneration, snapshot: snapshot})
	if read == nil || model.readGeneration != before+1 {
		t.Fatalf("editor return read = %T/generation %d, want command/%d", read, model.readGeneration, before+1)
	}
	model, _ = updateModelCommand(t, model, scanResult{generation: model.scanGeneration, snapshot: snapshot})
	if model.readGeneration != before+1 {
		t.Fatalf("repeated unchanged scan generation = %d, want %d", model.readGeneration, before+1)
	}
}

func TestModelActiveWatcherErrorSurvivesSuccessfulRestart(t *testing.T) {
	model := loadedModel(t)
	model.watchGeneration = 1
	changes := make(chan notes.Change)
	model, _ = updateModelCommand(t, model, watchResult{
		generation: model.watchGeneration,
		changes:    changes,
		change:     notes.Change{Err: assertError("events failed")},
		open:       true,
	})
	model = updateModel(t, model, watchStarted{generation: model.watchGeneration, changes: changes})
	if footer := model.statusView(120, ContextTree); !strings.Contains(footer, "watch: events failed") {
		t.Fatalf("status after watcher restart = %q, want persistent error", footer)
	}
}

type assertError string

func (e assertError) Error() string { return string(e) }

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func TestTreeReconcileDeletedSelectionFallsBackToNextSibling(t *testing.T) {
	tree, ok := NewTree(notes.Snapshot{Entries: []notes.Entry{
		entry("a.md", notes.KindMarkdown),
		entry("b.md", notes.KindMarkdown),
		entry("c.md", notes.KindMarkdown),
	}}).Select("b.md")
	if !ok {
		t.Fatal("Select() did not find b.md")
	}

	tree = tree.Reconcile(notes.Snapshot{Entries: []notes.Entry{
		entry("a.md", notes.KindMarkdown),
		entry("c.md", notes.KindMarkdown),
	}})

	assertSelectedPath(t, tree, "c.md")
}
