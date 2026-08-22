package ui

import (
	"testing"

	"github.com/gonfff/jotmd/internal/notes"
)

func TestTreeNavigatesVisibleEntriesWithDefaultKeys(t *testing.T) {
	tree := NewTree(testSnapshot())

	for _, tt := range []struct {
		key  string
		want notes.RelPath
	}{
		{key: "j", want: "docs/guide"},
		{key: "k", want: "docs"},
		{key: "down", want: "docs/guide"},
		{key: "up", want: "docs"},
		{key: "end", want: "note.md"},
		{key: "home", want: "docs"},
	} {
		action, ok := actionForKey(DefaultBindings(), tt.key, ContextTree)
		if !ok {
			t.Fatalf("actionForKey(%q) did not resolve", tt.key)
		}
		tree = tree.Update(action)
		entry, ok := tree.Selected()
		if !ok || entry.Path != tt.want {
			t.Errorf("after %q, Selected() = %q, want %q", tt.key, entry.Path, tt.want)
		}
	}
}

func TestTreeStartsWithDirectoriesExpanded(t *testing.T) {
	tree := NewTree(testSnapshot())
	visible := tree.visible()
	want := []notes.RelPath{"docs", "docs/guide", "docs/guide/intro.md", "docs/readme.md", "note.md"}
	if len(visible) != len(want) {
		t.Fatalf("visible entries = %d, want %d", len(visible), len(want))
	}
	for index, entry := range visible {
		if entry.Path != want[index] {
			t.Errorf("visible[%d] = %q, want %q", index, entry.Path, want[index])
		}
	}
}

func TestTreeExpandsCollapsesAndSelectsParent(t *testing.T) {
	tree := NewTree(testSnapshot())
	tree = tree.Update(ActionExpand)
	assertSelectedPath(t, tree, "docs")

	tree = tree.Update(ActionDown)
	assertSelectedPath(t, tree, "docs/guide")
	tree = tree.Update(ActionExpand)
	tree = tree.Update(ActionDown)
	assertSelectedPath(t, tree, "docs/guide/intro.md")

	tree = tree.Update(ActionParent)
	assertSelectedPath(t, tree, "docs/guide")
	tree = tree.Update(ActionCollapse)
	tree = tree.Update(ActionDown)
	assertSelectedPath(t, tree, "docs/readme.md")

	tree = tree.Update(ActionParent)
	assertSelectedPath(t, tree, "docs")
	tree = tree.Update(ActionCollapse)
	tree = tree.Update(ActionDown)
	assertSelectedPath(t, tree, "note.md")
}

func TestTreeCollapseHidesDescendants(t *testing.T) {
	tree := NewTree(testSnapshot()).Update(ActionDown)
	assertSelectedPath(t, tree, "docs/guide")

	tree = tree.Update(ActionCollapse)
	if _, ok := tree.Selected(); !ok {
		t.Fatal("collapsed directory selection disappeared")
	}
	if len(tree.visible()) != 4 {
		t.Fatalf("visible entries after collapse = %d, want 4", len(tree.visible()))
	}
	tree = tree.Update(ActionCollapse)
	assertSelectedPath(t, tree, "docs")
}

func TestTreeEnterTogglesDirectory(t *testing.T) {
	tree := NewTree(testSnapshot())
	tree = tree.Update(ActionOpen)
	tree = tree.Update(ActionOpen)
	tree = tree.Update(ActionDown)
	tree = tree.Update(ActionDown)
	assertSelectedPath(t, tree, "docs/guide/intro.md")

	tree = tree.Update(ActionParent).Update(ActionOpen).Update(ActionDown)
	assertSelectedPath(t, tree, "docs/readme.md")
}

func TestSpaceTogglesDirectory(t *testing.T) {
	model := testModel(t)
	model = updateModel(t, model, run(model.Init()))

	model = updateModel(t, model, key("space"))
	if model.tree.expanded["docs"] || model.fullPreview || model.focus != ContextTree {
		t.Fatalf("expanded/full preview/focus = (%t, %t, %q), want (false, false, tree)", model.tree.expanded["docs"], model.fullPreview, model.focus)
	}
	model = updateModel(t, model, key("space"))
	if !model.tree.expanded["docs"] || model.fullPreview {
		t.Fatalf("expanded/full preview = (%t, %t), want (true, false)", model.tree.expanded["docs"], model.fullPreview)
	}
}

func TestTreeReconcilePreservesSelectedPathAndExpansion(t *testing.T) {
	tree := NewTree(testSnapshot()).Update(ActionExpand).Update(ActionDown).Update(ActionExpand).Update(ActionDown)
	assertSelectedPath(t, tree, "docs/guide/intro.md")

	tree = tree.Reconcile(notes.Snapshot{Entries: []notes.Entry{
		entry("docs", notes.KindDirectory),
		entry("docs/guide", notes.KindDirectory),
		entry("docs/guide/intro.md", notes.KindMarkdown),
		entry("docs/new.md", notes.KindMarkdown),
		entry("note.md", notes.KindMarkdown),
	}})
	assertSelectedPath(t, tree, "docs/guide/intro.md")

	tree = tree.Update(ActionDown)
	assertSelectedPath(t, tree, "docs/new.md")
}

func TestTreeSelectExpandsParents(t *testing.T) {
	tree, ok := NewTree(testSnapshot()).Select("docs/guide/intro.md")
	if !ok {
		t.Fatal("Select() did not find nested note")
	}
	assertSelectedPath(t, tree, "docs/guide/intro.md")
	if !tree.expanded["docs"] || !tree.expanded["docs/guide"] {
		t.Fatalf("expanded = %#v, want selected note parents", tree.expanded)
	}
}

func TestTreeKeepsSelectionVisibleBelowViewport(t *testing.T) {
	tree := NewTree(notes.Snapshot{Entries: []notes.Entry{
		entry("one.md", notes.KindMarkdown),
		entry("two.md", notes.KindMarkdown),
		entry("three.md", notes.KindMarkdown),
		entry("four.md", notes.KindMarkdown),
	}}).SetViewport(2)

	tree = tree.Update(ActionDown).Update(ActionDown)
	if tree.viewport != 1 {
		t.Fatalf("viewport = %d, want 1", tree.viewport)
	}
	visible := tree.visible()
	if visible[tree.viewport].Path != "two.md" || visible[tree.viewport+1].Path != "three.md" {
		t.Fatalf("viewport entries = %q, %q, want two.md, three.md", visible[tree.viewport].Path, visible[tree.viewport+1].Path)
	}
}

func TestTreeEmptyViewportStaysAtZero(t *testing.T) {
	tree := NewTree(notes.Snapshot{}).SetViewport(4)
	if tree.viewport != 0 {
		t.Fatalf("empty tree viewport = %d, want 0", tree.viewport)
	}
}

func TestDefaultTreeBindingsContainNavigationActions(t *testing.T) {
	for _, binding := range DefaultBindings() {
		switch binding.Action {
		case ActionUp, ActionDown, ActionCollapse, ActionExpand, ActionOpen:
			if !hasContext(binding.Contexts, ContextTree) || !hasContext(binding.Contexts, ContextTOC) {
				t.Errorf("navigation binding %#v does not support tree and TOC", binding)
			}
		case ActionFirst, ActionLast, ActionParent:
			if len(binding.Contexts) != 1 || binding.Contexts[0] != ContextTree {
				t.Errorf("navigation binding %#v is not limited to tree context", binding)
			}
		}
	}
}

func testSnapshot() notes.Snapshot {
	return notes.Snapshot{Entries: []notes.Entry{
		entry("docs", notes.KindDirectory),
		entry("docs/guide", notes.KindDirectory),
		entry("docs/guide/intro.md", notes.KindMarkdown),
		entry("docs/readme.md", notes.KindMarkdown),
		entry("note.md", notes.KindMarkdown),
	}}
}

func entry(path notes.RelPath, kind notes.Kind) notes.Entry {
	name := string(path)
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			name = name[i+1:]
			break
		}
	}
	return notes.Entry{Path: path, Name: name, Kind: kind}
}

func assertSelectedPath(t *testing.T, tree Tree, want notes.RelPath) {
	t.Helper()
	got, ok := tree.Selected()
	if !ok || got.Path != want {
		t.Fatalf("Selected() = %q, want %q", got.Path, want)
	}
}
