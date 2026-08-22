package ui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/notes"
)

const (
	searchDelay        = 100 * time.Millisecond
	searchMaxFileBytes = 8 << 20
	searchMaxResults   = 200
)

type searchState struct {
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
	paths      []notes.Match
	matches    []notes.Match
	selected   int
	err        string
}

type searchReady struct {
	generation uint64
	query      string
}

type searchContentResult struct {
	generation uint64
	matches    []notes.Match
	err        error
}

func (m *Model) openSearch() {
	m.mode = SearchPrompt
	m.input = textinput.New()
	m.input.Prompt = "/ "
	m.input.Focus()
	m.search = searchState{}
}

func (m *Model) closeSearch() {
	if m.search.cancel != nil {
		m.search.cancel()
	}
	m.search.generation++
	m.search.paths = nil
	m.search.matches = nil
	m.input.Blur()
	m.mode = Browse
}

func (m Model) updateSearch(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch message.String() {
	case "esc":
		m.closeSearch()
		return m, nil
	case "up", "ctrl+p":
		if m.search.selected > 0 {
			m.search.selected--
		}
		return m, nil
	case "down", "ctrl+n":
		if m.search.selected+1 < len(m.search.matches) {
			m.search.selected++
		}
		return m, nil
	case "enter":
		if len(m.search.matches) == 0 {
			return m, nil
		}
		path := m.search.matches[m.search.selected].Path
		m.closeSearch()
		var selected bool
		m.tree, selected = m.tree.Select(path)
		if !selected {
			return m, nil
		}
		return m, m.readSelected()
	}
	m.input, _ = m.input.Update(message)
	return m, m.refreshSearch()
}

func (m *Model) refreshSearch() tea.Cmd {
	if m.search.cancel != nil {
		m.search.cancel()
	}
	m.search.generation++
	m.search.ctx, m.search.cancel = context.WithCancel(m.ctx)
	query := m.input.Value()
	m.search.paths = notes.RankPaths(m.tree.snapshot, query, searchMaxResults)
	m.search.matches = append([]notes.Match(nil), m.search.paths...)
	m.search.selected = 0
	m.search.err = ""
	if utf8.RuneCountInString(query) < 2 || len(m.search.paths) == searchMaxResults {
		return nil
	}
	generation := m.search.generation
	return tea.Tick(searchDelay, func(time.Time) tea.Msg {
		return searchReady{generation: generation, query: query}
	})
}

func (m Model) searchContentCommand(ctx context.Context, generation uint64, query string, limit int) tea.Cmd {
	return func() tea.Msg {
		matches, _, err := m.store.SearchContent(ctx, query, searchMaxFileBytes, limit)
		return searchContentResult{generation: generation, matches: matches, err: err}
	}
}

func (m Model) searchView(width, height int) string {
	type resultLine struct {
		text       string
		matchIndex int
	}
	lines := make([]resultLine, 0, len(m.search.matches)+3)
	headingStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.theme.Palette.Accent))
	for _, group := range []struct {
		title string
		kind  notes.MatchKind
	}{{title: "Paths", kind: notes.MatchPath}, {title: "Contents", kind: notes.MatchContent}} {
		groupMatches := 0
		for _, match := range m.search.matches {
			if match.Kind == group.kind {
				groupMatches++
			}
		}
		if groupMatches == 0 {
			continue
		}
		if len(lines) != 0 {
			lines = append(lines, resultLine{matchIndex: -1})
		}
		lines = append(lines, resultLine{text: headingStyle.Render(group.title), matchIndex: -1})
		for index, match := range m.search.matches {
			if match.Kind != group.kind {
				continue
			}
			label := string(match.Path)
			if match.Kind == notes.MatchContent {
				label = fmt.Sprintf("%s:%d  %s", match.Path, match.Line, match.Snippet)
			}
			lines = append(lines, resultLine{text: "  " + label, matchIndex: index})
		}
	}
	if len(m.search.matches) == 0 && m.input.Value() != "" {
		lines = append(lines, resultLine{text: "No matches.", matchIndex: -1})
	}
	selectedRow := 0
	for index, line := range lines {
		if line.matchIndex == m.search.selected {
			selectedRow = index
			break
		}
	}
	start := max(0, selectedRow-height+1)
	visible := make([]string, 0, height)
	selectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.SelectionForeground)).Background(lipgloss.Color(m.theme.Palette.SelectionBackground))
	for _, line := range lines[start:] {
		text := line.text
		if line.matchIndex == m.search.selected && line.matchIndex >= 0 {
			text = "> " + strings.TrimPrefix(text, "  ")
			text = ansi.Truncate(text, width, "")
			text += strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
			text = selectionStyle.Render(text)
		}
		visible = append(visible, text)
		if len(visible) == height {
			break
		}
	}
	return strings.Join(paddedLines(visible, width, height), "\n")
}

func (m Model) searchPopup(width, height int) string {
	if m.input.Value() == "" || height < 3 {
		return ""
	}
	rows := 1
	if len(m.search.matches) != 0 {
		rows = len(m.search.matches)
		pathMatches, contentMatches := 0, 0
		for _, match := range m.search.matches {
			if match.Kind == notes.MatchPath {
				pathMatches++
			} else if match.Kind == notes.MatchContent {
				contentMatches++
			}
		}
		if pathMatches > 0 {
			rows++
		}
		if contentMatches > 0 {
			rows++
		}
		if pathMatches > 0 && contentMatches > 0 {
			rows++
		}
	}
	outerHeight := min(height, min(10, rows+2))
	outerWidth := max(1, width-4)
	content := m.searchView(max(1, outerWidth-2), max(1, outerHeight-2))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.Palette.BorderFocus)).
		Background(lipgloss.Color(m.theme.Palette.Background)).
		Width(outerWidth).
		Height(outerHeight).
		Render(content)
}

func (m Model) searchStatus(width int) string {
	suffix := ""
	if m.search.err != "" {
		suffix = "  ! " + m.search.err
	}
	return statusInputLine("Search: ", m.input, suffix, width)
}
