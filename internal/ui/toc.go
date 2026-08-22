package ui

import (
	"bytes"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type tocEntry struct {
	level      int
	title      string
	line       int
	sourceLine int
}

func extractHeadings(source []byte) []tocEntry {
	document := goldmark.New().Parser().Parse(text.NewReader(source))
	var headings []tocEntry
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		heading, ok := node.(*ast.Heading)
		if entering && ok {
			title := string(heading.Text(source))
			if title == "" {
				return ast.WalkContinue, nil
			}
			headings = append(headings, tocEntry{
				level:      heading.Level,
				title:      title,
				sourceLine: bytes.Count(source[:heading.Lines().At(0).Start], []byte{'\n'}),
			})
		}
		return ast.WalkContinue, nil
	})
	return headings
}

func headingMarker(source []byte) string {
	marker := "\u2063"
	for bytes.Contains(source, []byte(marker)) {
		marker += "\u2063"
	}
	return marker
}

func markHeadings(source []byte, marker string) []byte {
	document := goldmark.New().Parser().Parse(text.NewReader(source))
	var offsets []int
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		heading, ok := node.(*ast.Heading)
		if entering && ok && len(heading.Text(source)) > 0 {
			offsets = append(offsets, heading.Lines().At(0).Start)
		}
		return ast.WalkContinue, nil
	})
	marked := make([]byte, 0, len(source)+len(offsets)*len(marker))
	previous := 0
	for _, offset := range offsets {
		marked = append(marked, source[previous:offset]...)
		marked = append(marked, marker...)
		previous = offset
	}
	return append(marked, source[previous:]...)
}

func mapHeadingLines(headings []tocEntry, rendered, marker string) []tocEntry {
	lines := strings.Split(rendered, "\n")
	mapped := append([]tocEntry(nil), headings...)
	preceding, next := 0, 0
	for line, content := range lines {
		if next == len(mapped) || !strings.Contains(content, marker) {
			continue
		}
		mapped[next].line = line
		preceding = line
		next++
	}
	for index := next; index < len(mapped); index++ {
		mapped[index].line = preceding
	}
	return mapped
}

func headingContinuation(line string) bool {
	return strings.HasPrefix(ansi.Strip(line), "↪ ")
}

func mapRawHeadingLines(headings []tocEntry, rendered string, wrapped bool) []tocEntry {
	mapped := append([]tocEntry(nil), headings...)
	if !wrapped {
		for index := range mapped {
			mapped[index].line = mapped[index].sourceLine
		}
		return mapped
	}

	visualLines := make([]int, 0)
	for index, line := range strings.Split(rendered, "\n") {
		if !headingContinuation(line) {
			visualLines = append(visualLines, index)
		}
	}
	preceding := 0
	for index := range mapped {
		mapped[index].line = preceding
		if mapped[index].sourceLine < len(visualLines) {
			mapped[index].line = visualLines[mapped[index].sourceLine]
			preceding = mapped[index].line
		}
	}
	return mapped
}

func (m Model) tocView(width, height int) []string {
	selected := m.preview.tocIndex
	if m.focus != ContextTOC {
		selected = m.preview.headingAtOffset()
	}
	selectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Palette.SelectionForeground)).
		Background(lipgloss.Color(m.theme.Palette.SelectionBackground))
	start := max(0, selected-height+1)
	lines := make([]string, 0, min(height, len(m.preview.toc)-start))
	for index := start; index < len(m.preview.toc) && len(lines) < height; index++ {
		entry := m.preview.toc[index]
		marker := "  "
		if index == selected {
			marker = "> "
		}
		line := marker + strings.Repeat("  ", max(0, entry.level-1)) + entry.title
		line = ansi.Truncate(line, width, "")
		if index == selected {
			line += strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
			line = selectionStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return paddedLines(lines, width, height)
}

func (p Preview) headingAtOffset() int {
	selected := -1
	for index, entry := range p.toc {
		if entry.line > p.viewport.YOffset() {
			break
		}
		selected = index
	}
	return selected
}

func (p *Preview) syncHeading() {
	if len(p.toc) == 0 {
		p.tocIndex = -1
		return
	}
	p.tocIndex = max(0, p.headingAtOffset())
}

func (p *Preview) jumpHeading(direction int) {
	if len(p.toc) == 0 {
		return
	}
	if p.tocIndex < 0 || p.tocIndex >= len(p.toc) {
		p.syncHeading()
	}
	p.tocIndex = min(max(0, p.tocIndex+direction), len(p.toc)-1)
	entry := p.toc[p.tocIndex]
	p.viewport.SetYOffset(entry.line)
}
