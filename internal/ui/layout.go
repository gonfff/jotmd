package ui

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/notes"
	"github.com/gonfff/jotmd/internal/theme"
)

const (
	wideWidth  = 90
	tinyWidth  = 40
	tinyHeight = 10
)

func previewWidth(width, percentage int) int {
	if width >= wideWidth {
		return width - treeWidth(width, percentage) - 4
	}
	return width - 4
}

func (m Model) previewWidthForLayout() int {
	return previewWidth(m.width, m.cfg.TreeWidth)
}

func treeWidth(width, percentage int) int {
	result := width * percentage / 100
	if result < 24 {
		return 24
	}
	if result > 48 {
		return 48
	}
	return result
}

func bodyHeight(height int, statusBar bool) int {
	reserved := 0
	if statusBar {
		reserved++
	}
	if height-reserved < 1 {
		return 1
	}
	return height - reserved
}

func paneHeight(height int, statusBar bool) int {
	return max(1, bodyHeight(height, statusBar)-2)
}

func (m Model) render() string {
	base := "Terminal too small (need 40×10)."
	if m.width >= tinyWidth && m.height >= tinyHeight {
		base = m.browseView()
	} else if m.mode == SearchPrompt || m.mode == ThemePicker || m.mode == RenderStylePicker {
		base = ansi.Truncate(base, m.width, "") + "\n" + m.statusView(m.width, m.focus)
	} else if isTextPrompt(m.mode) || isConfirmation(m.mode) {
		base = ansi.Truncate(base, m.width, "") + "\n" + m.statusView(m.width, m.focus)
	}
	popupWidth, popupHeight := m.popupSize()
	popup := m.popupContent(popupWidth, popupHeight)
	if popup == "" {
		return base
	}
	return overlay(base, popup, m.width, m.height)
}

func (m Model) popupSize() (int, int) {
	width, height := 78, 10
	if m.help {
		width, height = 96, 24
		return min(width, max(1, m.width-4)), min(height, max(1, m.height-2))
	}
	return min(width, max(1, m.width-4)), min(height, max(1, m.height-4))
}

func (m Model) popupContent(width, height int) string {
	var content string
	large := false
	switch {
	case m.help:
		content = m.helpView(width, height)
		large = true
	case m.mode == CommandPalette:
		content = m.paletteView(width, height)
		large = true
	}
	if content == "" {
		return ""
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.Palette.BorderFocus)).
		MaxWidth(width + 2).
		MaxHeight(height + 2)
	if large {
		style = style.Width(width + 2).Height(height + 2)
	}
	return style.Render(content)
}

func overlay(base, popup string, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	x := max(0, (width-lipgloss.Width(popup))/2)
	y := max(0, (height-lipgloss.Height(popup))/2)
	canvas := lipgloss.NewCanvas(width, height)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(popup).X(x).Y(y).Z(1),
	))
	return canvas.Render()
}

func overlayBottom(base, popup string, width, height int) string {
	if popup == "" || width < 1 || height < 1 {
		return base
	}
	x := max(0, (width-lipgloss.Width(popup))/2)
	y := max(0, height-lipgloss.Height(popup))
	canvas := lipgloss.NewCanvas(width, height)
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(popup).X(x).Y(y).Z(1),
	))
	return canvas.Render()
}

func (m Model) browseView() string {
	height := bodyHeight(m.height, m.statusBarVisible())
	var body string
	treeHeight := max(1, height-2)
	previewHeight := max(1, height-4)
	if m.fullPreview && m.width >= wideWidth {
		leftWidth := treeWidth(m.width, m.cfg.TreeWidth)
		rightWidth := m.width - leftWidth
		body = lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.pane(m.tocView(max(1, leftWidth-2), treeHeight), leftWidth, height, m.focus == ContextTOC, false),
			m.pane(m.previewView(max(1, rightWidth-4), previewHeight), rightWidth, height, m.focus == ContextPreview, true),
		)
	} else if m.fullPreview && m.focus == ContextTOC {
		body = m.pane(m.tocView(max(1, m.width-2), treeHeight), m.width, height, true, false)
	} else if m.fullPreview {
		body = m.pane(m.previewView(max(1, m.width-4), previewHeight), m.width, height, true, true)
	} else if m.width >= wideWidth {
		leftWidth := treeWidth(m.width, m.cfg.TreeWidth)
		rightWidth := m.width - leftWidth
		body = lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.pane(m.treeView(max(1, leftWidth-2), treeHeight), leftWidth, height, m.focus == ContextTree, false),
			m.pane(m.previewView(max(1, rightWidth-4), previewHeight), rightWidth, height, m.focus == ContextPreview, true),
		)
	} else if m.focus == ContextTree {
		body = m.pane(m.treeView(max(1, m.width-2), treeHeight), m.width, height, true, false)
	} else {
		body = m.pane(m.previewView(max(1, m.width-4), previewHeight), m.width, height, true, true)
	}
	switch m.mode {
	case SearchPrompt:
		body = overlayBottom(body, m.searchPopup(m.width, height), m.width, height)
	case ThemePicker:
		body = overlayBottom(body, m.themePickerPopup(m.width, height), m.width, height)
	case RenderStylePicker:
		body = overlayBottom(body, m.renderStylePickerPopup(m.width, height), m.width, height)
	}
	if !m.statusBarVisible() {
		return body
	}
	return body + "\n" + m.statusView(m.width, m.focus)
}

func (m Model) themePickerPopup(width, height int) string {
	return m.pickerPopup(width, height, len(m.themePicker.filtered), m.themePickerView)
}

func (m Model) renderStylePickerPopup(width, height int) string {
	return m.pickerPopup(width, height, len(m.renderStylePicker.filtered), m.renderStylePickerView)
}

func (m Model) pickerPopup(width, height, rows int, view func(int, int) string) string {
	if height < 3 {
		return ""
	}
	outerHeight := min(height, min(10, max(1, rows)+2))
	outerWidth := max(1, width-4)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.Palette.BorderFocus)).
		Width(outerWidth).
		Height(outerHeight).
		Render(view(max(1, outerWidth-2), max(1, outerHeight-2)))
}

func (m Model) pane(lines []string, width, height int, focused, padded bool) string {
	color := m.theme.Palette.Border
	border := lipgloss.RoundedBorder()
	if focused {
		color = m.theme.Palette.BorderFocus
		border = lipgloss.DoubleBorder()
	}
	style := lipgloss.NewStyle().
		Border(border).
		BorderForeground(lipgloss.Color(color)).
		Width(max(1, width)).
		Height(max(1, height))
	if padded {
		style = style.Padding(1)
	}
	return style.Render(strings.Join(lines, "\n"))
}

func (m Model) treeView(width, height int) []string {
	if !m.scanned {
		return paddedLines([]string{"Loading notes…"}, width, height)
	}
	entries := m.tree.visible()
	total := len(entries)
	if m.tree.viewport > 0 && m.tree.viewport < len(entries) {
		entries = entries[m.tree.viewport:]
	}
	if len(entries) == 0 {
		return paddedLines([]string{"No notes found."}, width, height)
	}
	lines := make([]string, 0, len(entries))
	selectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Palette.SelectionForeground)).
		Background(lipgloss.Color(m.theme.Palette.SelectionBackground))
	for _, entry := range entries {
		indent := strings.Repeat("  ", strings.Count(string(entry.Path), "/"))
		name := entry.Name
		if entry.Kind == notes.KindDirectory {
			marker := "▸"
			if m.tree.expanded[entry.Path] {
				marker = "▾"
			}
			name = marker + " " + name
		}
		marker := "  "
		if entry.Path == m.tree.selected {
			marker = "> "
		} else if entry.Kind == notes.KindDirectory {
			name = lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Directory)).Render(name)
		}
		line := marker + indent + name
		if entry.Path == m.tree.selected {
			line = ansi.Truncate(line, width, "")
			line += strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
			line = selectionStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return scrollLines(lines, width, height, total, m.tree.viewport)
}

func scrollLines(lines []string, width, height, total, offset int) []string {
	if total <= height {
		return paddedLines(lines, width, height)
	}
	result := paddedLines(lines, max(1, width-1), height)
	maxOffset := total - height
	thumb := offset * max(0, height-1) / max(1, maxOffset)
	for index := range result {
		marker := "│"
		if index == thumb {
			marker = "█"
		}
		if index == 0 && offset > 0 {
			marker = "▲"
		}
		if index == height-1 && offset < maxOffset {
			marker = "▼"
		}
		result[index] += marker
	}
	return result
}

func (m Model) previewView(width, height int) []string {
	selected, selectedOK := m.tree.Selected()
	if !m.hasDocument && (!selectedOK || selected.Kind != notes.KindDirectory) {
		if strings.HasPrefix(m.status, "too large to preview:") {
			return paddedLines([]string{m.status}, width, height)
		}
		return paddedLines([]string{"Select a note to preview."}, width, height)
	}
	content := m.preview.View()
	if m.cfg.NoColor {
		content = theme.StripStyles(content)
	}
	return scrollLines(
		strings.Split(strings.TrimSuffix(content, "\n"), "\n"),
		width,
		height,
		m.preview.viewport.TotalLineCount(),
		m.preview.viewport.YOffset(),
	)
}

func (m Model) directoryPreview(directory notes.Entry, width, height int) string {
	entries := m.directoryEntries(directory)
	if len(entries) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Muted)).Render("No notes in this directory.")
	}

	columns, cardWidth, cardHeight := directoryGridSize(width, height, len(entries))
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Foreground))
	contentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Palette.Muted))
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.Palette.Border)).
		Padding(0, 1).
		Width(cardWidth).
		Height(cardHeight)
	rows := make([]string, 0, (len(entries)+columns-1)/columns)
	for start := 0; start < len(entries); start += columns {
		cards := make([]string, 0, columns)
		for index := start; index < min(start+columns, len(entries)); index++ {
			excerpt, loaded := m.directoryExcerpts[entries[index].Path]
			if !loaded {
				excerpt = "Loading…"
			}
			innerWidth := max(1, cardWidth-4)
			body := strings.Split(ansi.Wrap(excerpt, innerWidth, ""), "\n")
			body = body[:min(len(body), max(1, cardHeight-3))]
			cards = append(cards, cardStyle.Render(
				titleStyle.Render(ansi.Truncate(entries[index].Name, innerWidth, ""))+"\n"+
					contentStyle.Render(strings.Join(body, "\n")),
			))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func directoryGridSize(width, height, entries int) (columns, cardWidth, cardHeight int) {
	columns = min(3, max(1, width/18))
	cardHeight = max(3, height/3)
	gridWidth := width
	if ((entries+columns-1)/columns)*cardHeight > height {
		gridWidth = max(1, gridWidth-1)
	}
	return columns, max(9, gridWidth/columns), cardHeight
}

func (m Model) directoryEntries(directory notes.Entry) []notes.Entry {
	prefix := string(directory.Path) + "/"
	entries := make([]notes.Entry, 0)
	for _, entry := range m.tree.snapshot.Entries {
		if entry.Kind == notes.KindMarkdown && strings.HasPrefix(string(entry.Path), prefix) {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(left, right int) bool {
		leftPath := strings.TrimPrefix(string(entries[left].Path), prefix)
		rightPath := strings.TrimPrefix(string(entries[right].Path), prefix)
		leftDepth, rightDepth := strings.Count(leftPath, "/"), strings.Count(rightPath, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return strings.ToLower(leftPath) < strings.ToLower(rightPath)
	})
	return entries
}

func noteExcerpt(content []byte) string {
	fallback := ""
	lines := make([]string, 0)
	for _, line := range strings.Split(strings.ToValidUTF8(string(content), "�"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if fallback == "" {
				fallback = strings.TrimSpace(strings.TrimLeft(line, "#"))
			}
			continue
		}
		lines = append(lines, line)
	}
	if fallback != "" {
		lines = append([]string{fallback}, lines...)
	}
	if len(lines) != 0 {
		return truncateRunes(strings.Join(lines, "\n"), 240)
	}
	return "Empty note"
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func paddedLines(lines []string, width, height int) []string {
	if width < 1 {
		width = 1
	}
	result := make([]string, 0, height)
	for _, line := range lines {
		if len(result) == height {
			break
		}
		line = ansi.Truncate(line, width, "")
		result = append(result, line)
	}
	for len(result) < height {
		result = append(result, "")
	}
	return result
}

func popupLines(lines []string, width, height int) string {
	return strings.TrimRight(strings.Join(paddedLines(lines, width, height), "\n"), "\n")
}
