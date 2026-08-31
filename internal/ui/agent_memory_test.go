package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/notes"
)

func TestAgentMemorySnapshotFilterMatchesReservedRootOnly(t *testing.T) {
	snapshot := notes.Snapshot{
		Root: "/vault",
		Entries: []notes.Entry{
			{Path: "agent-memory", Kind: notes.KindDirectory},
			{Path: "agent-memory/global", Kind: notes.KindDirectory},
			{Path: "agent-memory/global/topic.md", Kind: notes.KindMarkdown},
			{Path: "agent-memory-old.md", Kind: notes.KindMarkdown},
			{Path: "user.md", Kind: notes.KindMarkdown},
		},
	}

	visible := visibleSnapshot(snapshot, false)
	if visible.Root != "/vault" {
		t.Fatalf("visible root = %q, want /vault", visible.Root)
	}
	for _, path := range []notes.RelPath{"agent-memory", "agent-memory/global", "agent-memory/global/topic.md"} {
		if snapshotHasPath(visible, path) {
			t.Errorf("visible snapshot contains hidden path %q", path)
		}
	}
	for _, path := range []notes.RelPath{"agent-memory-old.md", "user.md"} {
		if !snapshotHasPath(visible, path) {
			t.Errorf("visible snapshot omits path %q", path)
		}
	}
	if all := visibleSnapshot(snapshot, true); len(all.Entries) != len(snapshot.Entries) {
		t.Fatalf("visible snapshot with agent memory = %d entries, want %d", len(all.Entries), len(snapshot.Entries))
	}
}

func TestModelHidesAgentMemoryFromTreeAndSearchByDefault(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "agent-memory", "global", "topic.md"), "memory-only needle\n")
	writeNote(t, filepath.Join(root, "agent-memory-old.md"), "ordinary user note\n")
	writeNote(t, filepath.Join(root, "user.md"), "user needle\n")
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Watch = false
	model := newModel(store, cfg, jotmdTheme(t))
	model = updateModel(t, model, run(model.Init()))

	if !snapshotHasPath(model.snapshot, "agent-memory/global/topic.md") {
		t.Fatal("full model snapshot omits agent-memory/global/topic.md")
	}
	for _, path := range []notes.RelPath{"agent-memory", "agent-memory/global", "agent-memory/global/topic.md"} {
		if snapshotHasPath(model.tree.snapshot, path) {
			t.Errorf("tree snapshot contains hidden path %q", path)
		}
	}
	if !snapshotHasPath(model.tree.snapshot, "agent-memory-old.md") {
		t.Fatal("tree snapshot omits agent-memory-old.md")
	}
	if matches := notes.RankPaths(model.tree.snapshot, "topic", searchMaxResults); len(matches) != 0 {
		t.Fatalf("path search returned hidden memory: %#v", matches)
	}
	result := run(model.searchContentCommand(context.Background(), 1, "needle", searchMaxResults)).(searchContentResult)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if len(result.matches) != 1 || result.matches[0].Path != "user.md" {
		t.Fatalf("content search = %#v, want only user.md", result.matches)
	}

	cfg.ShowAgentMemory = true
	visible := newModel(store, cfg, jotmdTheme(t))
	visible = updateModel(t, visible, run(visible.Init()))
	if !snapshotHasPath(visible.tree.snapshot, "agent-memory/global/topic.md") {
		t.Fatal("startup show_agent_memory does not include agent memory tree")
	}
}

func TestModelRescanKeepsAgentMemoryVisibility(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "agent-memory", "global", "topic.md"), "memory-only needle\n")
	writeNote(t, filepath.Join(root, "agent-memory-old.md"), "ordinary user note\n")
	writeNote(t, filepath.Join(root, "user.md"), "user needle\n")
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Watch = false
	model := newModel(store, cfg, jotmdTheme(t))
	model = updateModel(t, model, run(model.Init()))
	writeNote(t, filepath.Join(root, "agent-memory", "global", "later.md"), "later memory\n")
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	model = updateModel(t, model, scanResult{generation: model.scanGeneration, snapshot: snapshot})

	if !snapshotHasPath(model.snapshot, "agent-memory/global/later.md") {
		t.Fatal("full model snapshot omits later agent memory")
	}
	if snapshotHasPath(model.tree.snapshot, "agent-memory/global/later.md") {
		t.Fatal("tree snapshot includes later agent memory")
	}
}

func TestModelTogglesAgentMemoryAndClearsHiddenSelection(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "agent-memory", "global", "topic.md"), "memory\n")
	writeNote(t, filepath.Join(root, "user.md"), "user\n")
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Watch = false
	cfg.ShowAgentMemory = true
	model := newModel(store, cfg, jotmdTheme(t))
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
	model = updateModel(t, model, run(model.Init()))
	model.tree, _ = model.tree.Select("agent-memory/global/topic.md")
	model = updateModel(t, model, run(model.readSelected()))
	if !model.hasDocument || model.document.Path != "agent-memory/global/topic.md" {
		t.Fatalf("memory document = %q, want agent-memory/global/topic.md", model.document.Path)
	}

	model, command := updateModelCommand(t, model, key("a"))
	if snapshotHasPath(model.tree.snapshot, "agent-memory/global/topic.md") {
		t.Fatal("memory tree remains after hiding")
	}
	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "user.md" {
		t.Fatalf("selected after hiding = %#v, want user.md", selected)
	}
	if model.hasDocument || model.document.Path != "" {
		t.Fatalf("hidden memory preview remains before replacement read: %#v", model.document)
	}
	if model.status != "agent memory hidden" {
		t.Fatalf("hide status = %q, want agent memory hidden", model.status)
	}
	if command == nil {
		t.Fatal("hiding memory did not start replacement read")
	}

	model, _ = updateModelCommand(t, model, key("a"))
	if !snapshotHasPath(model.tree.snapshot, "agent-memory/global/topic.md") {
		t.Fatal("memory tree remains hidden after toggling visible")
	}
	if model.status != "agent memory visible" {
		t.Fatalf("show status = %q, want agent memory visible", model.status)
	}

	onlyMemory := t.TempDir()
	writeNote(t, filepath.Join(onlyMemory, "agent-memory", "global", "topic.md"), "memory\n")
	onlyMemoryStore, err := notes.NewStore(onlyMemory, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	empty := newModel(onlyMemoryStore, cfg, jotmdTheme(t))
	empty = updateModel(t, empty, tea.WindowSizeMsg{Width: 80, Height: 12})
	empty = updateModel(t, empty, run(empty.Init()))
	empty = updateModel(t, empty, key("a"))
	if len(empty.tree.snapshot.Entries) != 0 || !strings.Contains(empty.View().Content, "No notes found.") {
		t.Fatalf("memory-only vault after hiding = (%#v, %q), want empty state", empty.tree.snapshot.Entries, empty.View().Content)
	}
}

func TestModelConfigReloadOnlyOverridesChangedAgentMemorySetting(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{})
	model = updateModel(t, model, key("a"))
	if !model.showAgentMemory {
		t.Fatal("session toggle did not show agent memory")
	}

	falseConfig := config.Defaults()
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{Config: falseConfig, theme: jotmdTheme(t)}})
	if !model.showAgentMemory {
		t.Fatal("unchanged configured false overrode session-visible state")
	}

	trueConfig := config.Defaults()
	trueConfig.ShowAgentMemory = true
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{Config: trueConfig, theme: jotmdTheme(t)}})
	if !model.showAgentMemory {
		t.Fatal("changed configured true did not set runtime-visible state")
	}

	model = updateModel(t, model, key("a"))
	if model.showAgentMemory {
		t.Fatal("session toggle did not hide agent memory")
	}
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{Config: trueConfig, theme: jotmdTheme(t)}})
	if model.showAgentMemory {
		t.Fatal("unchanged configured true overrode session-hidden state")
	}
}

func TestModelConfigReloadHidingAgentMemoryClosesAndCancelsSearch(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "agent-memory", "global", "topic.md"), "memory needle\n")
	writeNote(t, filepath.Join(root, "user.md"), "user needle\n")
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Watch = false
	cfg.ShowAgentMemory = true
	model := newModel(store, cfg, jotmdTheme(t))
	model = updateModel(t, model, run(model.Init()))
	model.openSearch()
	model.input.SetValue("topic")
	model.refreshSearch()
	model.search.matches = []notes.Match{
		{Path: "agent-memory/global/topic.md", Kind: notes.MatchPath},
		{Path: "agent-memory/global/topic.md", Kind: notes.MatchContent},
	}
	canceled := false
	model.search.cancel = func() { canceled = true }
	generation := model.search.generation

	hidden := config.Defaults()
	hidden.Watch = false
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{Config: hidden, theme: jotmdTheme(t)}})

	if model.mode != Browse || !canceled || model.search.generation == generation {
		t.Fatalf("hidden search = (mode %v, canceled %t, generation %d), want closed and canceled", model.mode, canceled, model.search.generation)
	}
	if len(model.search.paths) != 0 || len(model.search.matches) != 0 {
		t.Fatalf("hidden search retained results: paths=%#v matches=%#v", model.search.paths, model.search.matches)
	}
	model = updateModel(t, model, searchContentResult{
		generation: generation,
		matches:    []notes.Match{{Path: "agent-memory/global/topic.md", Kind: notes.MatchContent}},
	})
	if model.mode != Browse || len(model.search.matches) != 0 {
		t.Fatalf("stale in-flight result reopened hidden memory: mode=%v matches=%#v", model.mode, model.search.matches)
	}

	model.openSearch()
	model.input.SetValue("user")
	model.refreshSearch()
	model = updateModel(t, model, searchContentResult{
		generation: generation,
		matches:    []notes.Match{{Path: "agent-memory/global/topic.md", Kind: notes.MatchContent}},
	})
	for _, match := range model.search.matches {
		if isAgentMemoryPath(match.Path) {
			t.Fatalf("delayed pre-hide result entered reopened search: generation=%d matches=%#v", model.search.generation, model.search.matches)
		}
	}
	if model.search.generation <= generation {
		t.Fatalf("reopened search generation = %d, want greater than pre-hide %d", model.search.generation, generation)
	}
}

func TestAgentMemoryActionIsDiscoverableAndCustomizable(t *testing.T) {
	for _, context := range []Context{ContextTree, ContextTOC, ContextPreview} {
		if action, ok := actionForKey(DefaultBindings(), "a", context); !ok || action != ActionAgentMemory {
			t.Fatalf("default a in %s = (%q, %t), want agent memory", context, action, ok)
		}
	}

	bindings := BindingsForKeymap(config.Keymap{Bindings: []config.Binding{{Action: "view.agent_memory", Keys: []string{"z"}}}})
	for _, context := range []Context{ContextTree, ContextTOC, ContextPreview} {
		if action, ok := actionForKey(bindings, "z", context); !ok || action != ActionAgentMemory {
			t.Fatalf("configured z in %s = (%q, %t), want agent memory", context, action, ok)
		}
		if _, ok := actionForKey(bindings, "a", context); ok {
			t.Fatalf("default a remains bound in %s after override", context)
		}
	}
	if !hasAction(ActionRegistry(), "view.agent_memory") {
		t.Fatal("view.agent_memory is not registered")
	}

	model := newConfiguredModel(t, config.Keymap{Bindings: []config.Binding{{Action: "view.agent_memory", Keys: []string{"z"}}}})
	if help := strings.Join(strings.Fields(ansi.Strip(strings.Join(model.helpLines(120), "\n"))), " "); !strings.Contains(help, "z agent memory") {
		t.Fatalf("help = %q, want configured agent memory binding", help)
	}
	model.openPalette()
	for _, item := range model.palette.items {
		if item.binding.Name == "view.agent_memory" && item.binding.Label == "agent memory" && strings.Join(item.binding.Keys, ",") == "z" {
			return
		}
	}
	t.Fatal("palette does not include configured agent memory binding")
}

func snapshotHasPath(snapshot notes.Snapshot, want notes.RelPath) bool {
	for _, entry := range snapshot.Entries {
		if entry.Path == want {
			return true
		}
	}
	return false
}
