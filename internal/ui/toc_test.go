package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/notes"
)

func TestTOCExtractsMarkdownHeadings(t *testing.T) {
	source := []byte(`# **Alpha**

## Duplicate

Setext *Title*
==============

~~~markdown
# Hidden
~~~

## Duplicate

### Unmatched
` + "\n## \n")
	want := []tocEntry{
		{level: 1, title: "Alpha", sourceLine: 0},
		{level: 2, title: "Duplicate", sourceLine: 2},
		{level: 1, title: "Setext Title", sourceLine: 4},
		{level: 2, title: "Duplicate", sourceLine: 11},
		{level: 3, title: "Unmatched", sourceLine: 13},
	}

	if got := extractHeadings(source); !reflect.DeepEqual(got, want) {
		t.Fatalf("extractHeadings() = %#v, want %#v", got, want)
	}
}

func TestTOCMapsRenderedHeadingsInsteadOfEarlierBodyMentions(t *testing.T) {
	headings := []tocEntry{
		{level: 1, title: "Alpha"},
		{level: 2, title: "Later Heading"},
	}
	marker := "\u2063"
	rendered := marker + "Alpha\nA paragraph mentions Later Heading before the section.\nLaterHeading\n" + marker + "Later Heading\nbody"
	want := []tocEntry{
		{level: 1, title: "Alpha", line: 0},
		{level: 2, title: "Later Heading", line: 3},
	}

	if got := mapHeadingLines(headings, rendered, marker); !reflect.DeepEqual(got, want) {
		t.Fatalf("mapHeadingLines() = %#v, want %#v", got, want)
	}
}

func TestTOCMapsRenderedHeadingAfterIdenticalBodyLine(t *testing.T) {
	preview := NewPreview()
	content := "# Alpha\n\nLater Heading\n\n## Later Heading\n"
	if err := preview.SetDocument(noteDocument(content), 80, 20, jotmdTheme(t)); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(preview.viewport.GetContent(), "\n")
	var matches []int
	for index, line := range lines {
		if strings.Join(strings.Fields(ansi.Strip(line)), " ") == "Later Heading" {
			matches = append(matches, index)
		}
	}
	if len(matches) != 2 {
		t.Fatalf("rendered Later Heading lines = %v; content=%q", matches, preview.viewport.GetContent())
	}
	if got := preview.toc[1].line; got != matches[1] {
		t.Fatalf("second TOC line = %d, want actual heading line %d (body line %d)", got, matches[1], matches[0])
	}
}

func TestTOCMapsRenderedHeadingSplitAcrossWrappedLines(t *testing.T) {
	headings := []tocEntry{
		{level: 1, title: "Alpha"},
		{level: 2, title: "A Heading Longer Than The Preview Width"},
		{level: 2, title: "Supercalifragilisticexpialidocious"},
	}
	marker := "\u2063"
	rendered := "  " + marker + "Alpha\n  body\n  " + marker + "A Heading Longer\n↪  Than The Preview\n↪  Width\n  body\n  " + marker + "Supercalifragil\n↪  isticexpialidocious\n  body"
	want := []tocEntry{
		{level: 1, title: "Alpha", line: 0},
		{level: 2, title: "A Heading Longer Than The Preview Width", line: 2},
		{level: 2, title: "Supercalifragilisticexpialidocious", line: 6},
	}

	if got := mapHeadingLines(headings, rendered, marker); !reflect.DeepEqual(got, want) {
		t.Fatalf("mapHeadingLines() = %#v, want %#v", got, want)
	}
}

func TestTOCMapsRawHeadingsBySourceLocationWithMentionsAndWrapping(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		width   int
		wrap    bool
		want    int
	}{
		{
			name:    "body mention before heading",
			content: "# Alpha\n\nLater Heading is mentioned before its section.\n\n## Later Heading\n",
			width:   80,
			wrap:    false,
			want:    4,
		},
		{
			name:    "heading wider than preview",
			content: "# Alpha\n\n## A Heading Longer Than The Preview Width\n",
			width:   16,
			wrap:    true,
			want:    2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			preview := NewPreview()
			preview.SetRaw(true)
			preview.SetWrap(test.wrap)
			if err := preview.SetDocument(noteDocument(test.content), test.width, 20, jotmdTheme(t)); err != nil {
				t.Fatal(err)
			}
			if len(preview.toc) != 2 || preview.toc[1].line != test.want {
				t.Fatalf("raw TOC = %#v, want second heading at line %d; content=%q", preview.toc, test.want, preview.viewport.GetContent())
			}
		})
	}
}

func TestTOCViewKeepsActiveHeadingVisibleAfterJumpingPastFirstPage(t *testing.T) {
	model := testModel(t)
	model.preview.SetContent(strings.Repeat("line\n", 80), 30, 3)
	for index := 0; index < 8; index++ {
		model.preview.toc = append(model.preview.toc, tocEntry{level: 1, title: "Heading " + string(rune('1'+index)), line: index * 8})
	}
	for range 5 {
		model.preview.jumpHeading(1)
	}

	view := strings.Join(model.tocView(20, 3), "\n")
	if !strings.Contains(view, "Heading 6") {
		t.Fatalf("TOC after heading jumps = %q, want active Heading 6 visible", view)
	}
}

func TestTOCMovesSelectionWhenShortPreviewCannotScroll(t *testing.T) {
	model := testModel(t)
	model.focus = ContextTOC
	model.preview.SetContent("first\nsecond\nthird", 30, 10)
	model.preview.toc = []tocEntry{
		{level: 1, title: "First", line: 0},
		{level: 1, title: "Second", line: 1},
		{level: 1, title: "Third", line: 2},
	}
	model.preview.tocIndex = 0

	model.preview.jumpHeading(1)
	if model.preview.tocIndex != 1 || model.preview.viewport.YOffset() != 0 {
		t.Fatalf("TOC index/offset = (%d, %d), want (1, 0)", model.preview.tocIndex, model.preview.viewport.YOffset())
	}
	if view := ansi.Strip(strings.Join(model.tocView(20, 3), "\n")); !strings.Contains(view, "> Second") {
		t.Fatalf("TOC view = %q, want selected marker on Second", view)
	}
}

func TestTOCMovesSelectionThroughLastVisibleHeadings(t *testing.T) {
	preview := NewPreview()
	preview.SetContent(strings.Repeat("line\n", 8), 30, 5)
	preview.toc = []tocEntry{
		{title: "First", line: 3},
		{title: "Second", line: 5},
		{title: "Third", line: 7},
	}
	preview.tocIndex = 0
	preview.viewport.SetYOffset(3)

	preview.jumpHeading(1)
	preview.jumpHeading(1)
	if preview.tocIndex != 2 || preview.viewport.YOffset() != 4 {
		t.Fatalf("TOC index/offset = (%d, %d), want last heading selected at maximum offset (2, 4)", preview.tocIndex, preview.viewport.YOffset())
	}
}

func TestTOCPlacesSelectedHeadingAtTopWhenSpaceRemains(t *testing.T) {
	preview := NewPreview()
	preview.SetContent(strings.Repeat("line\n", 20), 30, 5)
	preview.toc = []tocEntry{{title: "First", line: 2}, {title: "Second", line: 4}}
	preview.tocIndex = 0
	preview.viewport.SetYOffset(2)

	preview.jumpHeading(1)
	if preview.tocIndex != 1 || preview.viewport.YOffset() != 4 {
		t.Fatalf("TOC index/offset = (%d, %d), want selected heading at top (1, 4)", preview.tocIndex, preview.viewport.YOffset())
	}
}

func noteDocument(content string) notes.Document {
	return notes.Document{Path: "note.md", Revision: "toc-test", Content: []byte(content)}
}

func TestTOCMapsDuplicateAndMissingHeadingsForward(t *testing.T) {
	headings := []tocEntry{
		{level: 1, title: "Alpha"},
		{level: 2, title: "Duplicate"},
		{level: 1, title: "Setext Title"},
		{level: 2, title: "Duplicate"},
		{level: 3, title: "Unmatched"},
	}
	marker := "\u2063"
	rendered := marker + "\x1b[31mAlpha\x1b[0m\nbody\n" + marker + "Duplicate\nbody\n" + marker + "Setext Title\nbody\n" + marker + "Duplicate\nbody"
	want := []tocEntry{
		{level: 1, title: "Alpha", line: 0},
		{level: 2, title: "Duplicate", line: 2},
		{level: 1, title: "Setext Title", line: 4},
		{level: 2, title: "Duplicate", line: 6},
		{level: 3, title: "Unmatched", line: 6},
	}

	if got := mapHeadingLines(headings, rendered, marker); !reflect.DeepEqual(got, want) {
		t.Fatalf("mapHeadingLines() = %#v, want %#v", got, want)
	}
}
