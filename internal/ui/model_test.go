package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/notes"
	"github.com/gonfff/jotmd/internal/theme"
)

func TestModelInitialScanAndSelectionRead(t *testing.T) {
	model := testModel(t)
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
	model = updateModel(t, model, run(model.Init()))

	if !model.scanned {
		t.Fatal("initial scan did not complete")
	}
	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "docs" {
		t.Fatalf("selected = %q, want docs", selected.Path)
	}
	if model.tree.height != paneHeight(12, model.cfg.StatusBar) || model.tree.viewport != 0 {
		t.Fatalf("tree geometry = (%d, %d), want (%d, 0)", model.tree.height, model.tree.viewport, paneHeight(12, model.cfg.StatusBar))
	}

	model, command := updateModelCommand(t, model, key("j"))
	model = updateModel(t, model, run(command))
	if !model.hasDocument || model.document.Path != "docs/guide.md" {
		t.Fatalf("document = %q, want docs/guide.md", model.document.Path)
	}
}

func TestModelEmptyTreeResizesWithoutInvalidViewport(t *testing.T) {
	root := t.TempDir()
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	model := newModel(store, config.Defaults(), jotmdTheme(t))
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
	model = updateModel(t, model, run(model.Init()))
	if model.tree.viewport != 0 || !strings.Contains(model.View().Content, "No notes found.") {
		t.Fatalf("empty tree viewport/view = (%d, %q)", model.tree.viewport, model.View().Content)
	}
}

func TestModelCommandsUseExternalContext(t *testing.T) {
	model := testModel(t)
	ctx, cancel := context.WithCancel(context.Background())
	model.SetContext(ctx)
	cancel()
	result := run(model.Init()).(scanResult)
	if !errors.Is(result.err, context.Canceled) {
		t.Fatalf("initial scan error = %v, want context canceled", result.err)
	}
}

func TestModelSwitchesFocusAndForwardsPreviewKeys(t *testing.T) {
	model := loadedModel(t)
	model = updateModel(t, model, key("tab"))
	if model.focus != ContextPreview {
		t.Fatalf("focus = %q, want preview", model.focus)
	}
	model = updateModel(t, model, key("tab"))
	if model.focus != ContextTree {
		t.Fatalf("focus = %q, want tree", model.focus)
	}
}

func TestMouseClickFocusesPane(t *testing.T) {
	model := sizedLoadedModel(t)
	splitX := treeWidth(model.width, model.cfg.TreeWidth)

	model.focus = ContextPreview
	model = updateModel(t, model, tea.MouseClickMsg(tea.Mouse{X: 2, Y: 2, Button: tea.MouseLeft}))
	if model.focus != ContextTree {
		t.Fatalf("tree click focus = %q, want tree", model.focus)
	}

	model = updateModel(t, model, tea.MouseClickMsg(tea.Mouse{X: splitX, Y: 2, Button: tea.MouseLeft}))
	if model.focus != ContextPreview {
		t.Fatalf("preview click focus = %q, want preview", model.focus)
	}
}

func TestMouseClickSelectsFullPreviewPaneAndTOCHeading(t *testing.T) {
	model := sizedLoadedModel(t)
	model.fullPreview = true
	model.focus = ContextPreview
	model.preview.viewport.SetHeight(3)
	model.preview.viewport.SetContent(strings.Repeat("line\n", 20))
	model.preview.toc = []tocEntry{
		{title: "First", line: 0},
		{title: "Second", line: 6},
		{title: "Third", line: 12},
	}
	model.preview.tocIndex = 0

	model = updateModel(t, model, tea.MouseClickMsg(tea.Mouse{X: 2, Y: 2, Button: tea.MouseLeft}))
	if model.focus != ContextTOC || model.preview.tocIndex != 1 || model.preview.viewport.YOffset() != 6 {
		t.Fatalf("TOC click = (focus=%q, index=%d, offset=%d), want (toc, 1, 6)", model.focus, model.preview.tocIndex, model.preview.viewport.YOffset())
	}

	model = updateModel(t, model, tea.MouseClickMsg(tea.Mouse{
		X:      treeWidth(model.width, model.cfg.TreeWidth) + 2,
		Y:      2,
		Button: tea.MouseLeft,
	}))
	if model.focus != ContextPreview {
		t.Fatalf("preview click focus = %q, want preview", model.focus)
	}
}

func TestMouseClickSelectsTreeFile(t *testing.T) {
	model := sizedLoadedModel(t)
	model.tree, _ = model.tree.Select("docs")
	model.tree = model.tree.Update(ActionCollapse)

	var command tea.Cmd
	model, command = updateModelCommand(t, model, tea.MouseClickMsg(tea.Mouse{X: 2, Y: 2, Button: tea.MouseLeft}))
	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "note.md" || command == nil {
		t.Fatalf("file click = (selected=%q, command=%T), want note.md read", selected.Path, command)
	}
	model = updateModel(t, model, run(command))
	if !model.hasDocument || model.document.Path != "note.md" {
		t.Fatalf("clicked document = %#v", model.document)
	}
}

func TestMouseClickUsesVisibleTreeRowInNarrowLayout(t *testing.T) {
	model := sizedLoadedModel(t)
	model.width = 80
	entries := make([]notes.Entry, 8)
	for index := range entries {
		entries[index] = entry(notes.RelPath(fmt.Sprintf("note-%02d.md", index)), notes.KindMarkdown)
	}
	model.tree = NewTree(notes.Snapshot{Entries: entries}).SetViewport(3)
	model.tree.viewport = 4
	model.focus = ContextTree

	model = updateModel(t, model, tea.MouseClickMsg(tea.Mouse{X: 2, Y: 1, Button: tea.MouseLeft}))
	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "note-04.md" {
		t.Fatalf("narrow viewport click selected %q, want note-04.md", selected.Path)
	}
}

func TestMouseClickOnDirectoryCardSelectsTOCAndLoadsNote(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		writeNote(t, filepath.Join(root, "docs", name), "# "+name+"\n\nBody.\n")
	}
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{120, 80} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			cfg := config.Defaults()
			cfg.Watch = false
			model := newModel(store, cfg, jotmdTheme(t))
			model = updateModel(t, model, tea.WindowSizeMsg{Width: width, Height: 18})
			model = updateModel(t, model, run(model.Init()))
			if width < wideWidth {
				model.focus = ContextPreview
			}

			contentWidth := previewWidth(width, model.cfg.TreeWidth)
			cardWidth := contentWidth / 3
			contentX := 2
			if width >= wideWidth {
				contentX += treeWidth(width, model.cfg.TreeWidth)
			}
			model, command := updateModelCommand(t, model, tea.MouseClickMsg(tea.Mouse{
				X:      contentX + cardWidth + cardWidth/2,
				Y:      3,
				Button: tea.MouseLeft,
			}))
			selected, ok := model.tree.Selected()
			if !ok || selected.Path != "docs/b.md" || !model.tree.expanded["docs"] || command == nil {
				t.Fatalf("card click = (selected=%q expanded=%t command=%T)", selected.Path, model.tree.expanded["docs"], command)
			}
			model = updateModel(t, model, run(command))
			if !model.hasDocument || model.document.Path != "docs/b.md" || model.focus != ContextPreview {
				t.Fatalf("card document/focus = (%q, %t, %q)", model.document.Path, model.hasDocument, model.focus)
			}
		})
	}
}

func TestMouseClickOnScrolledDirectoryGridUsesViewportOffset(t *testing.T) {
	root := t.TempDir()
	for index := range 10 {
		writeNote(t, filepath.Join(root, "docs", fmt.Sprintf("note-%02d.md", index)), "Body.\n")
	}
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Watch = false
	model := newModel(store, cfg, jotmdTheme(t))
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 18})
	model = updateModel(t, model, run(model.Init()))
	model.preview.viewport.GotoBottom()

	width := previewWidth(model.width, model.cfg.TreeWidth)
	height := paneHeight(model.height, model.statusBarVisible()) - 2
	columns, cardWidth, cardHeight := directoryGridSize(width, height, 10)
	targetRow := 9 / columns
	visibleY := targetRow*cardHeight - model.preview.viewport.YOffset() + 1
	x := treeWidth(model.width, model.cfg.TreeWidth) + 2 + cardWidth/2
	model = updateModel(t, model, tea.MouseClickMsg(tea.Mouse{X: x, Y: 2 + visibleY, Button: tea.MouseLeft}))
	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "docs/note-09.md" {
		t.Fatalf("scrolled card click selected %q, want docs/note-09.md", selected.Path)
	}
}

func TestMouseWheelScrollsPaneUnderPointer(t *testing.T) {
	model := sizedLoadedModel(t)
	entries := make([]notes.Entry, 20)
	for index := range entries {
		entries[index] = entry(notes.RelPath(fmt.Sprintf("note-%02d.md", index)), notes.KindMarkdown)
	}
	model.tree = NewTree(notes.Snapshot{Entries: entries}).SetViewport(5)
	model.preview.viewport.SetHeight(3)
	model.preview.viewport.SetContent(strings.Repeat("preview\n", 30))
	model.focus = ContextPreview

	model = updateModel(t, model, tea.MouseWheelMsg(tea.Mouse{X: 2, Y: 2, Button: tea.MouseWheelDown}))
	if model.tree.viewport == 0 || model.preview.viewport.YOffset() != 0 || model.focus != ContextPreview {
		t.Fatalf("tree hover wheel = (tree=%d preview=%d focus=%q)", model.tree.viewport, model.preview.viewport.YOffset(), model.focus)
	}

	previewX := treeWidth(model.width, model.cfg.TreeWidth) + 2
	model.focus = ContextTree
	model = updateModel(t, model, tea.MouseWheelMsg(tea.Mouse{X: previewX, Y: 2, Button: tea.MouseWheelDown}))
	if model.preview.viewport.YOffset() == 0 || model.focus != ContextTree {
		t.Fatalf("preview hover wheel = (offset=%d focus=%q)", model.preview.viewport.YOffset(), model.focus)
	}
}

func TestMouseClickIgnoresBackgroundAndNonLeft(t *testing.T) {
	click := func(model Model, mouse tea.Mouse) Model {
		return updateModel(t, model, tea.MouseClickMsg(mouse))
	}
	model := sizedLoadedModel(t)
	model.focus = ContextPreview

	for _, test := range []struct {
		name  string
		model Model
		mouse tea.Mouse
	}{
		{name: "footer", model: model, mouse: tea.Mouse{X: 2, Y: bodyHeight(model.height, model.statusBarVisible()), Button: tea.MouseLeft}},
		{name: "non-left", model: model, mouse: tea.Mouse{X: 2, Y: 2, Button: tea.MouseRight}},
		{name: "help", model: func() Model { got := model; got.help = true; return got }(), mouse: tea.Mouse{X: 2, Y: 2, Button: tea.MouseLeft}},
		{name: "popup", model: func() Model { got := model; got.mode = ThemePicker; return got }(), mouse: tea.Mouse{X: 2, Y: 2, Button: tea.MouseLeft}},
		{name: "narrow", model: func() Model { got := model; got.width = wideWidth - 1; return got }(), mouse: tea.Mouse{X: 2, Y: 2, Button: tea.MouseLeft}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := click(test.model, test.mouse)
			if got.focus != test.model.focus {
				t.Fatalf("focus = %q, want %q", got.focus, test.model.focus)
			}
		})
	}
}

func TestModelOpensTOCBeforeFullPreview(t *testing.T) {
	for _, openKey := range []string{"enter", "space", "right", "l"} {
		t.Run(openKey, func(t *testing.T) {
			model := sizedLoadedModel(t)
			model, command := updateModelCommand(t, model, key(openKey))
			if !model.fullPreview || model.focus != ContextTOC {
				t.Fatalf("full preview/focus = (%t, %q), want (true, toc)", model.fullPreview, model.focus)
			}
			if command == nil || model.preview.current.width != model.previewWidthForLayout() {
				t.Fatalf("render command/width = (%v, %d), want command and %d", command != nil, model.preview.current.width, model.previewWidthForLayout())
			}
			if view := ansi.Strip(model.View().Content); strings.Contains(view, "docs") {
				t.Fatalf("full preview still shows tree: %q", view)
			}
		})
	}
}

func TestFullPreviewTOCUsesLeftPaneAndKeepsPreviewRight(t *testing.T) {
	model := sizedLoadedModel(t)
	model.width = 120
	model.document = notes.Document{Path: "note.md", Revision: "toc", Content: []byte("# Outline Title\n\n## Details\n")}
	model.preview.render = func(notes.Document, int, bool, theme.Theme, string) (string, error) {
		return "Rendered body", nil
	}

	model, command := updateModelCommand(t, model, key("enter"))
	model = updateModel(t, model, run(command))
	view := ansi.Strip(model.View().Content)
	lines := strings.Split(view, "\n")
	body := strings.Join(lines[:len(lines)-1], "\n")
	leftWidth := treeWidth(model.width, model.cfg.TreeWidth)
	if strings.Contains(body, "note.md") || strings.Contains(body, "guide.md") {
		t.Fatalf("full preview TOC still shows tree filenames: %q", view)
	}
	for _, want := range []string{"Outline Title", "Details"} {
		line := viewLineContaining(view, want)
		if line == "" || strings.Index(line, want) >= leftWidth {
			t.Fatalf("TOC heading %q is not in left pane: %q", want, view)
		}
	}
	bodyLine := viewLineContaining(view, "Rendered body")
	if bodyLine == "" || strings.Index(bodyLine, "Rendered body") < leftWidth {
		t.Fatalf("preview body is not in right pane: %q", view)
	}
}

func viewLineContaining(view, want string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	return ""
}

func TestFullPreviewNarrowMovesFromTOCToPreview(t *testing.T) {
	model := sizedLoadedModel(t)
	model.width = 80
	model.document = notes.Document{Path: "note.md", Revision: "narrow-toc", Content: []byte("# Narrow Outline\n")}
	model.preview.render = func(notes.Document, int, bool, theme.Theme, string) (string, error) {
		return "Rendered body", nil
	}

	model, command := updateModelCommand(t, model, key("enter"))
	model = updateModel(t, model, run(command))
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "Narrow Outline") || strings.Contains(view, "Rendered body") {
		t.Fatalf("narrow TOC = %q, want outline-only body", view)
	}
	model = updateModel(t, model, key("right"))
	view = ansi.Strip(model.View().Content)
	if strings.Contains(view, "Narrow Outline") || !strings.Contains(view, "Rendered body") {
		t.Fatalf("narrow preview = %q, want preview-only body", view)
	}
	if model.preview.current.width != model.width-4 {
		t.Fatalf("narrow preview width = %d, want %d", model.preview.current.width, model.width-4)
	}
}

func TestFocusedTOCMovesByHeading(t *testing.T) {
	model := sizedLoadedModel(t)
	model.fullPreview = true
	model.focus = ContextTOC
	model.preview.viewport.SetHeight(3)
	model.preview.viewport.SetContent(strings.Repeat("line\n", 20))
	model.preview.toc = []tocEntry{
		{level: 1, title: "First", line: 2},
		{level: 1, title: "Second", line: 6},
		{level: 1, title: "Third", line: 10},
	}
	model.preview.viewport.SetYOffset(2)

	model = updateModel(t, model, key("down"))
	if got := model.preview.viewport.YOffset(); got != 6 {
		t.Fatalf("Down offset = %d, want next heading at 6", got)
	}
	model = updateModel(t, model, key("up"))
	if got := model.preview.viewport.YOffset(); got != 2 {
		t.Fatalf("Up offset = %d, want previous heading at 2", got)
	}
	model = updateModel(t, model, key("j"))
	if got := model.preview.viewport.YOffset(); got != 6 {
		t.Fatalf("j offset = %d, want next heading at 6", got)
	}
	model = updateModel(t, model, key("k"))
	if got := model.preview.viewport.YOffset(); got != 2 {
		t.Fatalf("k offset = %d, want previous heading at 2", got)
	}
}

func TestFullPreviewMovesFocusBetweenTOCAndPreview(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("right"))

	model = updateModel(t, model, key("right"))
	if !model.fullPreview || model.focus != ContextPreview {
		t.Fatalf("after second Right full preview/focus = (%t, %q), want (true, preview)", model.fullPreview, model.focus)
	}
	model = updateModel(t, model, key("left"))
	if !model.fullPreview || model.focus != ContextTOC {
		t.Fatalf("after first Left full preview/focus = (%t, %q), want (true, toc)", model.fullPreview, model.focus)
	}
}

func TestModelReturnsFromFullPreview(t *testing.T) {
	for _, closeKey := range []string{"esc", "left", "h"} {
		t.Run(closeKey, func(t *testing.T) {
			model := sizedLoadedModel(t)
			model, command := updateModelCommand(t, model, key("enter"))
			model = updateModel(t, model, run(command))

			model, command = updateModelCommand(t, model, key(closeKey))
			if model.fullPreview || model.focus != ContextTree {
				t.Fatalf("full preview/focus = (%t, %q), want (false, tree)", model.fullPreview, model.focus)
			}
			if command == nil || model.preview.current.width != previewWidth(model.width, model.cfg.TreeWidth) {
				t.Fatalf("render command/width = (%v, %d), want command and %d", command != nil, model.preview.current.width, previewWidth(model.width, model.cfg.TreeWidth))
			}
		})
	}
}

func TestModelFullPreviewRoundTripPreservesScrollRatio(t *testing.T) {
	model := sizedLoadedModel(t)
	model.document = notes.Document{
		Path:     "note.md",
		Revision: "long",
		Content:  []byte(strings.Repeat("a long wrapped line ", 400)),
	}
	model = updateModel(t, model, run(model.renderDocument()))
	maximum := model.preview.viewport.TotalLineCount() - model.preview.viewport.VisibleLineCount()
	if maximum < 1 {
		t.Fatal("test document is not scrollable")
	}
	model.preview.viewport.SetYOffset(maximum * 9 / 10)
	want := model.preview.viewport.ScrollPercent()

	model, command := updateModelCommand(t, model, key("enter"))
	model = updateModel(t, model, run(command))
	model, command = updateModelCommand(t, model, key("esc"))
	model = updateModel(t, model, run(command))

	if got := model.preview.viewport.ScrollPercent(); got < want-0.05 || got > want+0.05 {
		t.Fatalf("scroll ratio after split/full/split = %.2f, want %.2f", got, want)
	}
}

func TestModelReloadReconcilesTree(t *testing.T) {
	model := loadedModel(t)
	model = updateModel(t, model, key("R"))
	if !model.loading {
		t.Fatal("reload did not start scan")
	}
}

func TestModelOpensAndClosesHelp(t *testing.T) {
	model := loadedModel(t)
	model = updateModel(t, model, key("?"))
	if !model.help {
		t.Fatal("help did not open")
	}
	model = updateModel(t, model, key("esc"))
	if model.help {
		t.Fatal("escape did not close help")
	}
}

func TestPopupEscapePreservesBrowseState(t *testing.T) {
	for _, test := range temporaryInterfaceCases() {
		t.Run(test.name, func(t *testing.T) {
			model := sizedLoadedModel(t)
			before, ok := model.tree.Selected()
			if !ok {
				t.Fatal("browse has no selected entry")
			}
			model = updateModel(t, model, key(test.key))
			model = updateModel(t, model, key("esc"))
			after, ok := model.tree.Selected()
			if !ok || after != before {
				t.Fatalf("selection after escape = %#v, want %#v", after, before)
			}
		})
	}
}

func TestPopupsInterceptMouseWheel(t *testing.T) {
	for _, test := range temporaryInterfaceCases() {
		t.Run(test.name, func(t *testing.T) {
			model := sizedLoadedModel(t)
			model.focus = ContextPreview
			model.preview.viewport.SetHeight(3)
			model.preview.viewport.SetContent(strings.Repeat("line\n", 50))
			model = updateModel(t, model, key(test.key))

			model = updateModel(t, model, tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))

			if offset := model.preview.viewport.YOffset(); offset != 0 {
				t.Fatalf("background preview offset = %d, want 0", offset)
			}
		})
	}
}

func temporaryInterfaceCases() []struct {
	name  string
	key   string
	title string
} {
	return []struct {
		name  string
		key   string
		title string
	}{
		{name: "commands", key: "P", title: "Commands"},
		{name: "themes", key: "t", title: "> jotmd"},
		{name: "trash", key: "d", title: "Move note.md to Trash?"},
	}
}

func TestModelQuits(t *testing.T) {
	model := loadedModel(t)
	_, command := updateModelCommand(t, model, key("q"))
	if _, ok := run(command).(tea.QuitMsg); !ok {
		t.Fatalf("q command = %T, want tea.QuitMsg", run(command))
	}
}

func TestModelShowsReadAndRenderErrors(t *testing.T) {
	model := loadedModel(t)
	model = updateModel(t, model, readResult{generation: model.readGeneration, path: "note.md", err: errors.New("read failed")})
	if !strings.Contains(model.status, "read failed") || model.hasDocument || strings.Contains(model.preview.View(), "Note") {
		t.Fatalf("status = %q, want read error", model.status)
	}
	model = loadedModel(t)
	model, command := updateModelCommand(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
	_ = command
	model = updateModel(t, model, PreviewRenderedMsg{Key: model.preview.current, Content: "safe fallback", Err: errors.New("render failed"), Rendered: true})
	if !strings.Contains(model.status, "render failed") || !model.hasDocument || !strings.Contains(model.preview.View(), "safe fallback") {
		t.Fatalf("render error state = (%q, %t, %q), want status and fallback", model.status, model.hasDocument, model.preview.View())
	}
	model = updateModel(t, model, PreviewRenderedMsg{Key: model.preview.current, Err: errors.New("render failed")})
	if model.hasDocument || model.preview.viewport.GetContent() != "" {
		t.Fatal("failed render without fallback did not clear document")
	}
}

func TestModelClearsDocumentAndCancelsReadWhenDirectorySelected(t *testing.T) {
	model := loadedModel(t)
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 18})
	canceled := false
	model.readCancel = func() { canceled = true }

	model = updateModel(t, model, key("k"))
	model = updateModel(t, model, key("k"))
	selected, ok := model.tree.Selected()
	if !ok || selected.Kind != notes.KindDirectory {
		t.Fatalf("selected = %#v, want directory", selected)
	}
	if model.hasDocument || model.document.Path != "" || model.document.Revision != "" || len(model.document.Content) != 0 {
		t.Fatalf("document was not cleared: %#v", model.document)
	}
	if content := model.preview.viewport.GetContent(); !strings.Contains(content, "guide.md") {
		t.Fatalf("directory preview = %q, want nested note card", content)
	}
	if !canceled {
		t.Fatal("pending read was not canceled")
	}
}

func TestModelClearsDocumentWhenReloadSelectsDirectory(t *testing.T) {
	model := loadedModel(t)
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 18})
	late := readResult{generation: model.readGeneration, path: model.document.Path, document: model.document}
	canceled := false
	model.readCancel = func() { canceled = true }

	model = updateModel(t, model, scanResult{
		generation: model.scanGeneration,
		snapshot: notes.Snapshot{Entries: []notes.Entry{
			entry("note.md", notes.KindDirectory),
		}},
	})
	if model.hasDocument || !strings.Contains(model.preview.viewport.GetContent(), "No notes") || !canceled {
		t.Fatalf("reload directory state = (%t, %q, %t), want empty directory preview and canceled read", model.hasDocument, model.preview.viewport.GetContent(), canceled)
	}
	model = updateModel(t, model, late)
	if model.hasDocument {
		t.Fatal("late read restored a document for a directory")
	}
}

func TestDirectoryPreviewIsRecursiveNearestFirstAndScrollable(t *testing.T) {
	root := t.TempDir()
	for index := 1; index <= 6; index++ {
		content := fmt.Sprintf("# Direct\n\nDirect body %02d.\n", index)
		if index == 1 {
			content = "# Card title\n\nFirst line.\nSecond line.\nA wrapped continuation that should split here.\nSentinel below card body limit.\n"
		}
		writeNote(t, filepath.Join(root, "docs", fmt.Sprintf("direct-%02d.md", index)), content)
	}
	for index := 1; index <= 3; index++ {
		writeNote(t, filepath.Join(root, "docs", "nested", fmt.Sprintf("nested-%02d.md", index)), "# Nested\n")
	}
	writeNote(t, filepath.Join(root, "docs", "nested", "deep", "deep.md"), "# Deep\n")
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Watch = false
	model := newModel(store, cfg, jotmdTheme(t))
	model = updateModel(t, model, run(model.Init()))
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 18})
	load := model.readSelected()
	if load == nil {
		t.Fatal("directory selection did not start loading card contents")
	}
	model = updateModel(t, model, run(load))

	content := model.preview.viewport.GetContent()
	if !strings.Contains(content, "Card title") {
		t.Fatalf("directory cards lack Markdown title: %q", content)
	}
	direct := strings.Index(content, "direct-06.md")
	nested := strings.Index(content, "nested-01.md")
	deep := strings.Index(content, "deep.md")
	if direct < 0 || nested < direct || deep < nested {
		t.Fatalf("directory card order is not nearest first: %q", content)
	}
	selected, ok := model.tree.Selected()
	if !ok {
		t.Fatal("directory is not selected")
	}
	for _, line := range strings.Split(model.directoryPreview(selected, 60, 12), "\n") {
		if width := ansi.StringWidth(line); width > 59 {
			t.Fatalf("scrollable card row width = %d, want room for scrollbar: %q", width, line)
		}
	}
	narrow := ansi.Strip(model.directoryPreview(selected, 18, 24))
	for _, want := range []string{"direct-01.md", "Card title", "First line.", "Second line.", "A wrapped", "continuation"} {
		if !strings.Contains(narrow, want) {
			t.Fatalf("narrow directory cards lack %q: %q", want, narrow)
		}
	}
	if strings.Contains(narrow, "Sentinel below") {
		t.Fatalf("narrow directory cards exceed the card body: %q", narrow)
	}
	visible := model.preview.View()
	if !strings.Contains(visible, "nested-03.md") || strings.Contains(visible, "deep.md") {
		t.Fatalf("first 3x3 page = %q, want first nine cards only", visible)
	}
	model.preview.viewport.GotoBottom()
	if visible = model.preview.View(); !strings.Contains(visible, "deep.md") {
		t.Fatalf("scrolled directory preview = %q, want deepest note", visible)
	}
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 16})
	if visible = model.preview.View(); !strings.Contains(visible, "deep.md") {
		t.Fatalf("resized directory preview = %q, want preserved scroll position", visible)
	}
}

func TestNoteExcerptKeepsMultipleContentLines(t *testing.T) {
	got := noteExcerpt([]byte("# Title\n\nFirst line.\nSecond line.\nThird line.\n"))
	if got != "Title\nFirst line.\nSecond line.\nThird line." {
		t.Fatalf("noteExcerpt() = %q", got)
	}
}

func TestDirectoryPreviewIgnoresLateContentAfterSelectingFile(t *testing.T) {
	model := sizedLoadedModel(t)
	model.tree, _ = model.tree.Select("docs")
	model.tree = model.tree.Update(ActionCollapse)
	model, load := updateModelCommand(t, model, key("j"))
	if load == nil {
		t.Fatal("directory selection did not start content load")
	}
	late := run(load)

	model, read := updateModelCommand(t, model, key("j"))
	model, render := updateModelCommand(t, model, run(read))
	model = updateModel(t, model, run(render))
	model = updateModel(t, model, late)
	if !model.hasDocument || model.document.Path != "note.md" || strings.Contains(model.preview.View(), "guide.md") {
		t.Fatalf("late directory content replaced selected file: document=%q preview=%q", model.document.Path, model.preview.View())
	}
}

func TestModelShowsTooLargePreviewStatus(t *testing.T) {
	model := loadedModel(t)
	limit := int64(10 * 1024 * 1024)
	model = updateModel(t, model, readResult{
		generation: model.readGeneration,
		path:       "note.md",
		err:        &notes.TooLargeError{Size: limit + 1, Limit: limit},
	})
	if model.hasDocument || !strings.Contains(model.status, "too large to preview: 10485761 bytes (limit 10485760 bytes)") {
		t.Fatalf("status = %q, want large preview status", model.status)
	}
	if !strings.Contains(strings.Join(model.previewView(80, 3), "\n"), "10485761 bytes") {
		t.Fatal("preview does not show large preview state")
	}
}

func TestModelIgnoresStaleGenerations(t *testing.T) {
	model := loadedModel(t)
	model, _ = updateModelCommand(t, model, key("R"))
	staleScan := scanResult{generation: model.scanGeneration - 1}
	model = updateModel(t, model, staleScan)
	if !model.loading {
		t.Fatal("stale scan ended current loading state")
	}

	current := model.document
	model.readGeneration++
	model = updateModel(t, model, readResult{generation: model.readGeneration - 1, path: current.Path, err: errors.New("stale read failed")})
	if model.document.Path != current.Path || model.document.Revision != current.Revision || model.status != "" {
		t.Fatalf("stale same-path read changed model: %#v", model)
	}
}

func TestModelOnlyCurrentRenderCompletesScrollRestore(t *testing.T) {
	model := sizedLoadedModel(t)
	model.preview.viewport.SetHeight(3)
	model.preview.viewport.SetContent(strings.Repeat("current\n", 20))
	model.restoreRatio = true
	model.previewRatio = 0.5
	before := model.preview.viewport.YOffset()
	staleKey := model.preview.current
	staleKey.width++

	model = updateModel(t, model, PreviewRenderedMsg{Key: staleKey, Content: "stale", Rendered: true})
	if !model.restoreRatio || model.preview.viewport.YOffset() != before {
		t.Fatalf("stale render completed restore = (%t, %d), want pending at offset %d", model.restoreRatio, model.preview.viewport.YOffset(), before)
	}

	model = updateModel(t, model, PreviewRenderedMsg{Key: model.preview.current, Content: strings.Repeat("current\n", 20), Rendered: true})
	if model.restoreRatio {
		t.Fatal("current render left scroll restore pending")
	}
	if got := model.preview.viewport.ScrollPercent(); got < 0.45 || got > 0.55 {
		t.Fatalf("current render restored ratio %.2f, want 0.50", got)
	}
}

func TestClearDocumentDropsPendingScrollRestore(t *testing.T) {
	model := sizedLoadedModel(t)
	model.preview.SetRaw(true)
	model.document = notes.Document{Path: "note.md", Revision: "old", Content: []byte(strings.Repeat("old\n", 50))}
	model = updateModel(t, model, run(model.renderDocument()))
	maximum := model.preview.viewport.TotalLineCount() - model.preview.viewport.VisibleLineCount()
	if maximum < 1 {
		t.Fatal("old document is not scrollable")
	}
	model.preview.viewport.SetYOffset(maximum * 9 / 10)
	if command := model.setFullPreview(true); command == nil {
		t.Fatal("full preview did not schedule pending render")
	}

	var selected bool
	model.tree, selected = model.tree.Select("docs")
	if !selected {
		t.Fatal("directory was not selected")
	}
	model.readSelected()
	model.tree, selected = model.tree.Select("docs/guide.md")
	if !selected {
		t.Fatal("new Markdown was not selected")
	}
	model.readSelected()
	model, render := updateModelCommand(t, model, readResult{
		generation: model.readGeneration,
		path:       "docs/guide.md",
		document:   notes.Document{Path: "docs/guide.md", Revision: "new", Content: []byte(strings.Repeat("new\n", 50))},
	})
	if render == nil {
		t.Fatal("new Markdown did not schedule a render")
	}
	model = updateModel(t, model, run(render))

	if offset := model.preview.viewport.YOffset(); offset != 0 {
		t.Fatalf("new document offset = %d, want top", offset)
	}
}

func TestModelIgnoresLateReadForPreviousSelection(t *testing.T) {
	model := loadedModel(t)
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})

	model, _ = updateModelCommand(t, model, key("k"))
	model, _ = updateModelCommand(t, model, key("k"))
	model = updateModel(t, model, key("l"))
	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "docs" {
		t.Fatalf("selected = %q, want docs", selected.Path)
	}
	model, firstRead := updateModelCommand(t, model, key("j"))
	firstPath := notes.RelPath("docs/guide.md")
	if firstRead == nil {
		t.Fatal("selecting note did not request a read")
	}

	model, secondRead := updateModelCommand(t, model, key("j"))
	secondPath := notes.RelPath("note.md")
	if secondRead == nil {
		t.Fatal("selecting note did not request a read")
	}
	first := run(firstRead).(readResult)
	if first.path != firstPath {
		t.Fatalf("first read path = %q, want %q", first.path, firstPath)
	}
	second := run(secondRead).(readResult)
	if second.path != secondPath {
		t.Fatalf("second read path = %q, want %q", second.path, secondPath)
	}

	model = updateModel(t, model, second)
	first.err = errors.New("stale read failed")
	model = updateModel(t, model, first)
	if model.document.Path != secondPath || strings.Contains(model.preview.View(), "Guide") || model.status != "" {
		t.Fatalf("late read replaced current preview: %#v", model.document)
	}
}

func TestModelResizeRerendersDocument(t *testing.T) {
	model := loadedModel(t)
	if !model.hasDocument {
		t.Fatal("loaded model has no document")
	}
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 20})
	if model.width != 100 || model.height != 20 {
		t.Fatalf("size = %dx%d, want 100x20", model.width, model.height)
	}
	if model.preview.current.width != previewWidth(100, model.cfg.TreeWidth) {
		t.Fatalf("preview width = %d, want %d", model.preview.current.width, previewWidth(100, model.cfg.TreeWidth))
	}
}

func TestModelLayoutsPreserveFocusedPane(t *testing.T) {
	model := loadedModel(t)
	for _, test := range []struct {
		name   string
		width  int
		height int
	}{
		{name: "wide", width: 100, height: 12},
		{name: "narrow", width: 80, height: 12},
		{name: "tiny", width: 39, height: 9},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, command := updateModelCommand(t, model, tea.WindowSizeMsg{Width: test.width, Height: test.height})
			if command != nil {
				got = updateModel(t, got, run(command))
			}
			view := ansi.Strip(got.View().Content)
			if test.width < tinyWidth || test.height < tinyHeight {
				if !strings.Contains(view, "Terminal too small") {
					t.Fatalf("tiny layout = %q", view)
				}
				return
			}
			if strings.HasPrefix(view, "jotmd\n") {
				t.Fatalf("layout retains header: %q", view)
			}
		})
	}
}

func TestTreeViewHighlightsSelectedRow(t *testing.T) {
	model := sizedLoadedModel(t)
	lines := model.treeView(20, 3)
	var selected string
	for _, line := range lines {
		if strings.HasPrefix(ansi.Strip(line), "> ") {
			selected = line
			break
		}
	}
	if selected == "" {
		t.Fatal("tree view has no selected row")
	}
	if ansi.StringWidth(ansi.Strip(selected)) != 20 {
		t.Fatalf("selected row width = %d, want 20", ansi.StringWidth(ansi.Strip(selected)))
	}
	name := strings.Index(selected, "note.md")
	if name < 0 || strings.Contains(selected[:name], "\x1b[0m") {
		t.Fatalf("selected row is not a contiguous selection span: %q", selected)
	}

	model.cfg.NoColor = true
	if view := model.View().Content; !strings.Contains(view, "> ") {
		t.Fatalf("no-color view loses selection marker: %q", view)
	}
}

func TestModelUsesConfiguredKeymapForDispatchAndHelp(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{Bindings: []config.Binding{
		{Action: "tree.down", Keys: []string{"n"}},
	}})
	model = updateModel(t, model, key("n"))
	selected, ok := model.tree.Selected()
	if !ok || selected.Path != "docs/guide.md" {
		t.Fatalf("configured key selected %q, want docs/guide.md", selected.Path)
	}
	if got := ansi.Strip(model.statusView(120, ContextTree)); ansi.StringWidth(got) != 120 || !strings.HasSuffix(got, "? help  ") {
		t.Fatalf("footer = %q, want inset ? help", got)
	}
}

func TestModelUsesConfiguredTreeNavigationInTOC(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{Bindings: []config.Binding{
		{Action: "tree.down", Keys: []string{"n"}},
	}})
	model.fullPreview = true
	model.focus = ContextTOC
	model.preview.viewport.SetHeight(3)
	model.preview.viewport.SetContent(strings.Repeat("line\n", 12))
	model.preview.toc = []tocEntry{{title: "First", line: 2}, {title: "Second", line: 6}}
	model.preview.viewport.SetYOffset(2)

	model = updateModel(t, model, key("n"))
	if got := model.preview.viewport.YOffset(); got != 6 {
		t.Fatalf("configured tree.down TOC offset = %d, want 6", got)
	}
}

func TestLayoutIndependentHotkeyUsesPhysicalBaseCode(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{})
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'т', BaseCode: 'n', Text: "т"}))
	if model.mode != NewNotePrompt {
		t.Fatalf("mode = %v, want new note prompt", model.mode)
	}
}

func TestLayoutIndependentHotkeysCanonicalizeShiftedPhysicalKeys(t *testing.T) {
	tests := []struct {
		name    string
		message tea.KeyPressMsg
		check   func(Model) bool
	}{
		{
			name:    "uppercase letter",
			message: tea.KeyPressMsg(tea.Key{Code: 'т', ShiftedCode: 'Т', BaseCode: 'n', Text: "Т", Mod: tea.ModShift}),
			check:   func(model Model) bool { return model.mode == NewDirectoryPrompt },
		},
		{
			name:    "question mark",
			message: tea.KeyPressMsg(tea.Key{Code: '.', ShiftedCode: ',', BaseCode: '/', Text: ",", Mod: tea.ModShift}),
			check:   func(model Model) bool { return model.help },
		},
		{
			name:    "command palette",
			message: tea.KeyPressMsg(tea.Key{Code: 'з', ShiftedCode: 'З', BaseCode: 'p', Text: "З", Mod: tea.ModShift}),
			check:   func(model Model) bool { return model.mode == CommandPalette },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := updateModel(t, newConfiguredModel(t, config.Keymap{}), test.message)
			if !test.check(model) {
				t.Fatalf("physical key %q did not dispatch its command", test.message.Key().BaseCode)
			}
		})
	}
}

func TestLayoutIndependentHotkeysWorkOutsideBrowseDispatch(t *testing.T) {
	t.Run("help scrolling", func(t *testing.T) {
		model := sizedLoadedModel(t)
		model.width, model.height = 80, 10
		model = updateModel(t, model, key("?"))
		model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'о', BaseCode: 'j', Text: "о"}))
		if model.helpOffset == 0 {
			t.Fatal("physical j did not scroll help")
		}
	})

	t.Run("full preview back", func(t *testing.T) {
		model := sizedLoadedModel(t)
		model.fullPreview = true
		model.focus = ContextPreview
		model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'р', BaseCode: 'h', Text: "р"}))
		if !model.fullPreview || model.focus != ContextTOC {
			t.Fatalf("physical h full preview/focus = (%t, %q), want (true, toc)", model.fullPreview, model.focus)
		}
		model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'р', BaseCode: 'h', Text: "р"}))
		if model.fullPreview || model.focus != ContextTree {
			t.Fatalf("second physical h full preview/focus = (%t, %q), want (false, tree)", model.fullPreview, model.focus)
		}
	})

	t.Run("search command palette", func(t *testing.T) {
		model := updateModel(t, sizedLoadedModel(t), key("/"))
		model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'з', ShiftedCode: 'З', BaseCode: 'p', Text: "З", Mod: tea.ModShift}))
		if model.mode != CommandPalette {
			t.Fatalf("mode = %v, want command palette", model.mode)
		}
	})

	t.Run("theme picker navigation", func(t *testing.T) {
		model := newConfiguredModel(t, config.Keymap{})
		model.themes = append(model.themes, model.theme)
		model.openThemePicker()
		model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'о', BaseCode: 'j', Text: "о"}))
		if model.themePicker.input.Value() != "о" {
			t.Fatalf("theme query = %q, want о", model.themePicker.input.Value())
		}
	})
}

func TestLayoutIndependentHotkeysPreserveLogicalText(t *testing.T) {
	t.Run("exact configured binding wins", func(t *testing.T) {
		model := newConfiguredModel(t, config.Keymap{Bindings: []config.Binding{
			{Action: "note.new", Keys: []string{"й"}},
		}})
		model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'й', BaseCode: 'q', Text: "й"}))
		if model.mode != NewNotePrompt {
			t.Fatalf("mode = %v, want exact Cyrillic binding", model.mode)
		}
	})

	t.Run("search receives active layout text", func(t *testing.T) {
		model := updateModel(t, sizedLoadedModel(t), key("/"))
		model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'т', BaseCode: 'n', Text: "т"}))
		if got := model.input.Value(); got != "т" {
			t.Fatalf("search input = %q, want Cyrillic text", got)
		}
	})
}

func TestLayoutIndependentFullPreviewPrefersExactBinding(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{Bindings: []config.Binding{
		{Action: "note.new", Keys: []string{"р"}},
	}})
	model.fullPreview = true
	model.focus = ContextPreview
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'р', BaseCode: 'h', Text: "р"}))
	if model.mode != NewNotePrompt {
		t.Fatalf("mode = %v, want exact Cyrillic binding", model.mode)
	}
}

func TestTreeViewShowsScrollThumb(t *testing.T) {
	entries := make([]notes.Entry, 8)
	for index := range entries {
		entries[index] = entry(notes.RelPath(fmt.Sprintf("%d.md", index)), notes.KindMarkdown)
	}
	model := testModel(t)
	model.scanned = true
	model.tree = NewTree(notes.Snapshot{Entries: entries}).SetViewport(3).Update(ActionDown).Update(ActionDown)
	view := strings.Join(model.treeView(20, 3), "\n")
	if !strings.Contains(view, "█") {
		t.Fatalf("tree view has no scroll thumb: %q", view)
	}
}

func TestPreviewViewShowsScrollThumb(t *testing.T) {
	model := testModel(t)
	model.hasDocument = true
	model.preview.viewport.SetWidth(19)
	model.preview.viewport.SetHeight(3)
	model.preview.viewport.SetContent("one\ntwo\nthree\nfour\nfive")
	model.preview.viewport.ScrollDown(1)
	view := strings.Join(model.previewView(20, 3), "\n")
	if !strings.Contains(view, "█") {
		t.Fatalf("preview view has no scroll thumb: %q", view)
	}
}

func loadedModel(t *testing.T) Model {
	t.Helper()
	model := testModel(t)
	model = updateModel(t, model, run(model.Init()))
	model, command := updateModelCommand(t, model, key("j"))
	model = updateModel(t, model, run(command))
	model, command = updateModelCommand(t, model, key("j"))
	return updateModel(t, model, run(command))
}

func testModel(t *testing.T) Model {
	t.Helper()
	cfg := config.Defaults()
	cfg.Watch = false
	return newModel(testStore(t), cfg, jotmdTheme(t))
}

func newModel(store *notes.Store, cfg config.Config, th theme.Theme) Model {
	return NewModelWithOptions(store, cfg, th, config.Keymap{}, []theme.Theme{th})
}

func newConfiguredModel(t *testing.T, keymap config.Keymap) Model {
	t.Helper()
	cfg := config.Defaults()
	cfg.Watch = false
	model := NewModelWithOptions(testStore(t), cfg, jotmdTheme(t), keymap, nil)
	return updateModel(t, model, run(model.Init()))
}

func testStore(t *testing.T) *notes.Store {
	t.Helper()
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "docs", "guide.md"), "# Guide\n")
	writeNote(t, filepath.Join(root, "note.md"), "# Note\n\nA preview.\n")
	store, err := notes.NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func writeNote(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func updateModel(t *testing.T, model Model, message tea.Msg) Model {
	t.Helper()
	next, _ := model.Update(message)
	return next.(Model)
}

func updateModelCommand(t *testing.T, model Model, message tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, command := model.Update(message)
	return next.(Model), command
}

func run(command tea.Cmd) tea.Msg {
	if command == nil {
		return nil
	}
	return command()
}

func key(value string) tea.KeyPressMsg {
	switch value {
	case "tab":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "up":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	case "down":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	case "backspace":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace})
	case "ctrl+p":
		return tea.KeyPressMsg(tea.Key{Code: 'p', Mod: tea.ModCtrl})
	case "ctrl+n":
		return tea.KeyPressMsg(tea.Key{Code: 'n', Mod: tea.ModCtrl})
	case "space":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeySpace})
	case "left":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft})
	case "right":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyRight})
	default:
		return tea.KeyPressMsg(tea.Key{Text: value})
	}
}
