package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/notes"
)

func TestSearchShowsPathResultsSynchronouslyAndDebouncesContent(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("/"))
	if model.mode != SearchPrompt {
		t.Fatalf("/ mode = %v, want SearchPrompt", model.mode)
	}

	model, delayed := updateModelCommand(t, model, key("note"))
	if len(model.search.matches) == 0 || model.search.matches[0].Path != "note.md" || model.search.matches[0].Kind != notes.MatchPath {
		t.Fatalf("synchronous path matches = %#v", model.search.matches)
	}
	if delayed == nil {
		t.Fatal("two-character query did not schedule delayed content search")
	}
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "Search") || !strings.Contains(view, "note.md") {
		t.Fatalf("search view = %q", view)
	}

	generation := model.search.generation
	model = updateModel(t, model, key("x"))
	model = updateModel(t, model, searchContentResult{
		generation: generation,
		matches:    []notes.Match{{Path: "stale.md", Kind: notes.MatchContent}},
	})
	if strings.Contains(model.searchView(model.width, model.height), "stale.md") {
		t.Fatal("stale search generation updated results")
	}
}

func TestSearchMinimumContentQueryUsesUnicodeCharacters(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("/"))
	model, command := updateModelCommand(t, model, key("я"))
	if command != nil {
		t.Fatal("one Unicode character scheduled content search")
	}
	model, command = updateModelCommand(t, model, key("д"))
	if command == nil {
		t.Fatal("two Unicode characters did not schedule content search")
	}
}

func TestSearchFixedNavigationOpensSelectionAndEscapeCancels(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("/"))
	model.search.matches = []notes.Match{
		{Path: "docs", Kind: notes.MatchPath},
		{Path: "note.md", Kind: notes.MatchPath},
	}

	for _, navigation := range []tea.KeyPressMsg{key("down"), ctrlKey('n'), key("up"), ctrlKey('p'), key("down")} {
		model = updateModel(t, model, navigation)
	}
	model, read := updateModelCommand(t, model, key("enter"))
	selected, ok := model.tree.Selected()
	if model.mode != Browse || !ok || selected.Path != "note.md" || read == nil {
		t.Fatalf("search open = (mode %v, selected %q, command %T)", model.mode, selected.Path, read)
	}

	model = updateModel(t, model, key("/"))
	canceled := false
	model.search.cancel = func() { canceled = true }
	model = updateModel(t, model, key("esc"))
	if model.mode != Browse || !canceled {
		t.Fatalf("escape = (mode %v, canceled %t)", model.mode, canceled)
	}
}

func TestSearchDirectoryExitsFullPreview(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("enter"))
	model = updateModel(t, model, key("/"))
	model.search.matches = []notes.Match{{Path: "docs", Kind: notes.MatchPath}}

	model = updateModel(t, model, key("enter"))

	selected, ok := model.tree.Selected()
	if !ok || selected.Kind != notes.KindDirectory || model.fullPreview || model.focus != ContextTree || model.hasDocument {
		t.Fatalf("directory selection = (%#v, full=%t, focus=%q, document=%t), want tree layout without document", selected, model.fullPreview, model.focus, model.hasDocument)
	}
}

func TestSearchContentResultKeepsPathMatchesFirst(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("/"))
	model = updateModel(t, model, key("note"))
	generation := model.search.generation
	model = updateModel(t, model, searchContentResult{
		generation: generation,
		matches: []notes.Match{
			{Path: "docs/guide.md", Kind: notes.MatchContent, Line: 1, Snippet: "note body"},
		},
	})
	seenContent := false
	for _, match := range model.search.matches {
		if match.Kind == notes.MatchContent {
			seenContent = true
		} else if seenContent {
			t.Fatalf("path result followed content result: %#v", model.search.matches)
		}
	}
}

func TestSearchUsesStatusBarAndSeparatesPathFromContentResults(t *testing.T) {
	model := sizedLoadedModel(t)
	model.cfg.StatusBar = false
	model = updateModel(t, model, key("/"))
	model.input.SetValue("note")
	model.search.matches = []notes.Match{
		{Path: "note.md", Kind: notes.MatchPath},
		{Path: "docs/guide.md", Kind: notes.MatchContent, Line: 2, Snippet: "note body"},
	}

	if popup := model.popupContent(78, 10); popup != "" {
		t.Fatalf("popupContent() = %q, want search outside popup", popup)
	}
	results := ansi.Strip(model.searchView(model.width, bodyHeight(model.height, true)))
	paths := strings.Index(results, "Paths")
	contents := strings.Index(results, "Contents")
	if paths < 0 || contents <= paths || !strings.Contains(results[paths:contents], "note.md") || !strings.Contains(results[contents:], "docs/guide.md:2") {
		t.Fatalf("search results are not separated:\n%s", results)
	}
	status := ansi.Strip(model.statusView(model.width, model.focus))
	if !model.statusBarVisible() || ansi.StringWidth(status) != model.width || !strings.Contains(status, "Search") || !strings.Contains(status, "note") {
		t.Fatalf("search status = %q, visible=%t", status, model.statusBarVisible())
	}
}

func TestSearchResultsGrowUpwardOverBrowseView(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("/"))
	model.input.SetValue("note")
	model.search.matches = []notes.Match{{Path: "note.md", Kind: notes.MatchPath}}

	small := strings.Split(ansi.Strip(model.View().Content), "\n")
	smallStart := lineContaining(small, "Paths")
	if smallStart <= 1 || !strings.Contains(strings.Join(small[:smallStart], "\n"), "docs") || lineContaining(small, "Contents") >= 0 {
		t.Fatalf("search does not overlay the browse view from below:\n%s", strings.Join(small, "\n"))
	}

	for index := 0; index < 6; index++ {
		model.search.matches = append(model.search.matches, notes.Match{Path: notes.RelPath(fmt.Sprintf("result-%d.md", index)), Kind: notes.MatchPath})
	}
	large := strings.Split(ansi.Strip(model.View().Content), "\n")
	largeStart := lineContaining(large, "Paths")
	if largeStart < 1 || largeStart >= smallStart {
		t.Fatalf("search popup did not grow upward: small=%d large=%d\n%s", smallStart, largeStart, strings.Join(large, "\n"))
	}
}

func lineContaining(lines []string, text string) int {
	for index, line := range lines {
		if strings.Contains(line, text) {
			return index
		}
	}
	return -1
}

func TestSearchReadyRunsContentAndErrorsRenderInStatusBar(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("/"))
	model = updateModel(t, model, key("preview"))
	generation := model.search.generation
	model, command := updateModelCommand(t, model, searchReady{generation: generation, query: "preview"})
	if command == nil {
		t.Fatal("current delayed generation did not start content search")
	}
	model = updateModel(t, model, run(command))
	if len(model.search.matches) == 0 || model.search.matches[len(model.search.matches)-1].Kind != notes.MatchContent {
		t.Fatalf("content result = %#v", model.search.matches)
	}

	model = updateModel(t, model, searchContentResult{generation: generation, err: errors.New("read failed")})
	if view := ansi.Strip(model.statusView(model.width, model.focus)); !strings.Contains(view, "read failed") {
		t.Fatalf("search error is absent from status bar: %q", view)
	}
}

func TestWatchRescanDoesNotCancelActiveSearch(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("/"))
	model = updateModel(t, model, key("note"))
	searchContext := model.search.ctx
	model.cfg.Watch = true
	_ = model.startWatch(model.tree.snapshot)
	if searchContext.Err() != nil || model.mode != SearchPrompt {
		t.Fatalf("watch restart canceled search: err=%v mode=%v", searchContext.Err(), model.mode)
	}
}

func TestSearchCanceledContextCannotStartContent(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("/"))
	model = updateModel(t, model, key("note"))
	model.closeSearch()
	model, command := updateModelCommand(t, model, searchReady{generation: model.search.generation - 1, query: "note"})
	if command != nil || model.mode != Browse {
		t.Fatalf("late debounce command = %T in mode %v", command, model.mode)
	}
	if model.search.ctx != nil && !errorsIsCanceled(model.search.ctx) {
		t.Fatal("closed search retained live context")
	}
}

func errorsIsCanceled(ctx context.Context) bool { return ctx.Err() == context.Canceled }
