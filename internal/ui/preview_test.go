package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/notes"
	"github.com/gonfff/jotmd/internal/theme"
)

func TestPreviewScrollsWithoutChangingDocument(t *testing.T) {
	preview := NewPreview()
	doc := notes.Document{Path: "note.md", Revision: "one", Content: []byte("# Title\n\nline one\nline two\nline three\n")}
	if err := preview.SetDocument(doc, 40, 2, jotmdTheme(t)); err != nil {
		t.Fatal(err)
	}
	content := preview.viewport.GetContent()

	preview.viewport.ScrollDown(1)
	if preview.viewport.YOffset() == 0 {
		t.Error("preview did not scroll")
	}
	if preview.viewport.GetContent() != content {
		t.Error("preview content changed while scrolling")
	}
}

func TestLayoutIndependentPreviewNavigationUsesPhysicalKey(t *testing.T) {
	model := testModel(t)
	model.focus = ContextPreview
	model.preview.SetContent("one\ntwo\nthree\nfour", 20, 2)
	model = updateModel(t, model, tea.KeyPressMsg(tea.Key{Code: 'о', BaseCode: 'j', Text: "о"}))
	if model.preview.viewport.YOffset() == 0 {
		t.Fatal("physical j did not scroll preview")
	}
}

func TestPreviewRerendersWhenWidthOrRenderInputsChange(t *testing.T) {
	preview := NewPreview()
	doc := notes.Document{Path: "note.md", Revision: "one", Content: []byte("# Heading\n\nA paragraph that wraps differently at narrow widths.\n")}
	th := jotmdTheme(t)
	if err := preview.SetDocument(doc, 80, 4, th); err != nil {
		t.Fatal(err)
	}
	wide := preview.viewport.GetContent()
	if err := preview.SetDocument(doc, 20, 4, th); err != nil {
		t.Fatal(err)
	}
	if preview.viewport.Width() != 20 || preview.viewport.GetContent() == wide {
		t.Error("width change did not rerender preview")
	}
	alternateTheme := th
	alternateTheme.Palette.Heading = "#FFFFFF"
	if err := preview.SetDocument(doc, 20, 4, alternateTheme); err != nil {
		t.Fatal(err)
	}

	changed := doc
	changed.Revision = "two"
	changed.Content = []byte("# Replacement\n")
	if err := preview.SetDocument(changed, 20, 4, th); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.viewport.GetContent(), "Replacement") {
		t.Error("revision change reused stale rendered content")
	}
	if len(preview.cache) != 1 {
		t.Errorf("cache has %d entries, want only the latest render", len(preview.cache))
	}
	preview.SetWrap(false)
	if err := preview.SetDocument(doc, 20, 4, alternateTheme); err != nil {
		t.Fatal(err)
	}
	if len(preview.cache) != 1 {
		t.Errorf("cache has %d entries after wrap change, want only the latest render", len(preview.cache))
	}
	preview.SetRenderStyle(theme.RenderStyleStructural)
	if err := preview.SetDocument(doc, 20, 4, alternateTheme); err != nil {
		t.Fatal(err)
	}
	if preview.current.renderStyle != theme.RenderStyleStructural || !strings.Contains(preview.viewport.GetContent(), "◆") {
		t.Fatal("render style change reused stale preview")
	}
}

func TestPreviewResetsScrollOnlyForDifferentRevision(t *testing.T) {
	preview := NewPreview()
	preview.render = func(doc notes.Document, _ int, _ bool, _ theme.Theme, _ string) (string, error) {
		return string(doc.Content), nil
	}
	doc := notes.Document{Revision: "one", Content: []byte("one\ntwo\nthree\nfour\nfive")}
	if err := preview.SetDocument(doc, 20, 2, jotmdTheme(t)); err != nil {
		t.Fatal(err)
	}
	preview.viewport.ScrollDown(2)

	if err := preview.SetDocument(doc, 19, 2, jotmdTheme(t)); err != nil {
		t.Fatal(err)
	}
	if preview.viewport.YOffset() == 0 {
		t.Fatal("same revision rerender reset scroll")
	}

	doc.Revision = "two"
	if err := preview.SetDocument(doc, 19, 2, jotmdTheme(t)); err != nil {
		t.Fatal(err)
	}
	if preview.viewport.YOffset() != 0 {
		t.Errorf("new revision offset = %d, want 0", preview.viewport.YOffset())
	}
}

func TestPreviewRejectsDocumentsOverConfiguredLimitBeforeRendering(t *testing.T) {
	preview := NewPreview()
	preview.SetMaxBytes(3)
	err := preview.SetDocument(notes.Document{Content: []byte("four")}, 20, 2, jotmdTheme(t))
	if !errors.Is(err, ErrDocumentTooLarge) {
		t.Errorf("SetDocument() error = %v, want %v", err, ErrDocumentTooLarge)
	}
}

func TestPreviewRenderCommandReturnsRenderedDocumentMessage(t *testing.T) {
	preview := NewPreview()
	message := preview.RenderCommand(notes.Document{Revision: "one", Content: []byte("# Title\n")}, 20, 2, jotmdTheme(t))()
	result, ok := message.(PreviewRenderedMsg)
	if !ok {
		t.Fatalf("RenderCommand() message = %T, want PreviewRenderedMsg", message)
	}
	if result.Err != nil || !strings.Contains(result.Content, "Title") {
		t.Errorf("PreviewRenderedMsg = %#v, want rendered title without error", result)
	}
}

func TestPreviewUpdateAppliesOnlyCurrentRenderMessage(t *testing.T) {
	preview := NewPreview()
	th := jotmdTheme(t)
	first := preview.RenderCommand(notes.Document{Revision: "one", Content: []byte("# First\n")}, 20, 2, th)
	second := preview.RenderCommand(notes.Document{Revision: "two", Content: []byte("# Second\n")}, 20, 2, th)

	if _, err := preview.Update(first()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(preview.View(), "First") {
		t.Error("stale render message replaced current preview")
	}
	if len(preview.toc) != 0 {
		t.Fatalf("stale render populated current TOC: %#v", preview.toc)
	}
	if _, err := preview.Update(second()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(preview.View(), "Second") {
		t.Error("current render message did not update preview")
	}
	if len(preview.toc) != 1 || preview.toc[0].title != "Second" {
		t.Fatalf("current render TOC = %#v, want Second", preview.toc)
	}
}

func TestPreviewTogglesRaw(t *testing.T) {
	model := sizedLoadedModel(t)
	model.document.Content = []byte{'#', ' ', 0xff, '\n'}

	model, command := updateModelCommand(t, model, key("v"))
	model = updateModel(t, model, run(command))
	if !model.preview.raw || model.preview.viewport.GetContent() != "# �\n" {
		t.Fatalf("raw/content = (%t, %q), want (true, replacement-safe source)", model.preview.raw, model.preview.viewport.GetContent())
	}

	model, command = updateModelCommand(t, model, key("v"))
	model = updateModel(t, model, run(command))
	if model.preview.raw || model.preview.viewport.GetContent() == "# �\n" {
		t.Fatalf("raw/content = (%t, %q), want rendered content", model.preview.raw, model.preview.viewport.GetContent())
	}
}

func TestPreviewTogglesWrap(t *testing.T) {
	model := sizedLoadedModel(t)
	model, command := updateModelCommand(t, model, key("v"))
	model = updateModel(t, model, run(command))
	if !model.preview.wrap || model.preview.viewport.SoftWrap {
		t.Fatalf("wrap/soft wrap = (%t, %t), want materialized wrapping", model.preview.wrap, model.preview.viewport.SoftWrap)
	}

	model, command = updateModelCommand(t, model, key("w"))
	model = updateModel(t, model, run(command))
	if model.preview.wrap || model.preview.viewport.SoftWrap {
		t.Fatalf("wrap/soft wrap = (%t, %t), want disabled wrapping", model.preview.wrap, model.preview.viewport.SoftWrap)
	}
}

func TestPreviewRawWrapKeepsFirstLineAtOriginalColumn(t *testing.T) {
	preview := NewPreview()
	preview.SetRaw(true)
	doc := notes.Document{Revision: "one", Content: []byte("alpha béta 世界 gamma delta\nomega")}
	if err := preview.SetDocument(doc, 12, 20, jotmdTheme(t)); err != nil {
		t.Fatal(err)
	}
	assertPreviewWrapContinuations(t, preview.viewport.GetContent(), 12, []string{"alpha", "béta", "世界", "gamma", "delta"})
	if got := preview.viewport.GetContent(); !strings.HasPrefix(got, "alpha") || !strings.Contains(got, "\nomega") {
		t.Fatalf("wrapped raw content shifts a first line: %q", got)
	}

	preview.SetWrap(false)
	if err := preview.SetDocument(doc, 12, 20, jotmdTheme(t)); err != nil {
		t.Fatal(err)
	}
	if got := preview.viewport.GetContent(); got != string(doc.Content) {
		t.Fatalf("unwrapped raw content = %q, want %q", got, doc.Content)
	}
}

func TestPreviewRenderedWrapKeepsFirstLineAtOriginalColumnAndPreservesANSI(t *testing.T) {
	preview := NewPreview()
	doc := notes.Document{Revision: "one", Content: []byte("**alpha béta 世界 gamma delta**")}
	if err := preview.SetDocument(doc, 12, 20, jotmdTheme(t)); err != nil {
		t.Fatal(err)
	}
	wrapped := preview.viewport.GetContent()
	if !strings.Contains(wrapped, "\x1b[") {
		t.Fatal("rendered preview lacks ANSI styling")
	}
	if !utf8.ValidString(wrapped) {
		t.Fatal("rendered preview contains invalid UTF-8")
	}
	assertPreviewWrapContinuations(t, wrapped, 12, []string{"alpha", "béta", "世界", "gamma", "delta"})

	preview.SetWrap(false)
	if err := preview.SetDocument(doc, 12, 20, jotmdTheme(t)); err != nil {
		t.Fatal(err)
	}
	unwrapped := ansi.Strip(preview.viewport.GetContent())
	if strings.Contains(unwrapped, "↪ ") || strings.HasPrefix(strings.TrimLeft(unwrapped, "\n"), "  ") {
		t.Fatalf("unwrapped rendered content has continuation gutter: %q", unwrapped)
	}
}

func TestPreviewWrapNeverOverflowsNarrowWidths(t *testing.T) {
	for _, width := range []int{1, 2} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			for index, line := range strings.Split(wrapContinuations("alpha", width), "\n") {
				if got := ansi.StringWidth(line); got > width {
					t.Errorf("line %d width = %d, want at most %d: %q", index, got, width, line)
				}
			}
		})
	}
}

func TestPreviewWrapNeverOverflowsDoubleWidthGrapheme(t *testing.T) {
	for _, width := range []int{1, 2} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			for index, line := range strings.Split(wrapContinuations("世", width), "\n") {
				if got := ansi.StringWidth(line); got > width {
					t.Errorf("line %d width = %d, want at most %d: %q", index, got, width, line)
				}
			}
		})
	}
}

func TestPreviewWrapNeverOverflowsWideContinuation(t *testing.T) {
	wrapped := wrapContinuations("aa世", 3)
	for index, line := range strings.Split(wrapped, "\n") {
		if got := ansi.StringWidth(line); got > 3 {
			t.Errorf("line %d width = %d, want at most 3: %q", index, got, line)
		}
	}
	if !strings.Contains(ansi.Strip(wrapped), "世") {
		t.Fatalf("wrapped content lost wide continuation: %q", wrapped)
	}
}

func assertPreviewWrapContinuations(t *testing.T, content string, width int, ordered []string) {
	t.Helper()
	continuations := 0
	for index, line := range strings.Split(content, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Errorf("line %d width = %d, want at most %d: %q", index, got, width, line)
		}
		plainLine := ansi.Strip(line)
		if index == 0 && (strings.HasPrefix(plainLine, "  ") || strings.HasPrefix(plainLine, "↪ ")) {
			t.Errorf("first line has wrap gutter: %q", line)
		}
		if strings.HasPrefix(plainLine, "↪ ") {
			continuations++
		}
	}
	if continuations == 0 {
		t.Error("wrapped preview has no continuation marker")
	}

	plain := ansi.Strip(content)
	position := 0
	for _, want := range ordered {
		index := strings.Index(plain[position:], want)
		if index < 0 {
			t.Fatalf("wrapped content = %q, want %q after byte %d", plain, want, position)
		}
		position += index + len(want)
	}
}

func TestPreviewIgnoresRenderFromPreviousMode(t *testing.T) {
	preview := NewPreview()
	doc := notes.Document{Revision: "one", Content: []byte("# Source\n")}
	stale := preview.RenderCommand(doc, 20, 2, jotmdTheme(t))
	preview.SetRaw(true)

	if _, err := preview.Update(stale()); err != nil {
		t.Fatal(err)
	}
	if content := preview.viewport.GetContent(); content != "" {
		t.Fatalf("stale rendered content = %q, want ignored", content)
	}

	current := preview.RenderCommand(doc, 20, 2, jotmdTheme(t))
	if _, err := preview.Update(current()); err != nil {
		t.Fatal(err)
	}
	if content := preview.viewport.GetContent(); content != "# Source\n" {
		t.Fatalf("raw content = %q, want source", content)
	}
}

func TestPreviewUpdateAppliesRendererFallbackAndReturnsError(t *testing.T) {
	preview := NewPreview()
	wantErr := errors.New("render failed")
	preview.render = func(notes.Document, int, bool, theme.Theme, string) (string, error) {
		return "safe fallback", wantErr
	}

	command := preview.RenderCommand(notes.Document{Revision: "one", Content: []byte("source")}, 20, 2, jotmdTheme(t))
	_, err := preview.Update(command())
	if !errors.Is(err, wantErr) {
		t.Errorf("Update() error = %v, want %v", err, wantErr)
	}
	if !strings.Contains(preview.View(), "safe fallback") {
		t.Error("renderer fallback was not applied to preview")
	}
}

func jotmdTheme(t *testing.T) theme.Theme {
	t.Helper()
	th, err := theme.Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	return th
}
