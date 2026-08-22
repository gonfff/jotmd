package ui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/notes"
	"github.com/gonfff/jotmd/internal/theme"
)

var ErrDocumentTooLarge = errors.New("document exceeds preview size limit")

type Preview struct {
	viewport       viewport.Model
	cache          map[renderKey]string
	maxBytes       int64
	wrap           bool
	raw            bool
	renderStyle    string
	current        renderKey
	hasCurrent     bool
	sourceHeadings []tocEntry
	toc            []tocEntry
	tocIndex       int
	render         func(notes.Document, int, bool, theme.Theme, string) (string, error)
}

type renderKey struct {
	revision    notes.Revision
	width       int
	theme       theme.Theme
	wrap        bool
	raw         bool
	renderStyle string
	tocMarker   string
}

type PreviewRenderedMsg struct {
	Key      renderKey
	Content  string
	Err      error
	Rendered bool
}

func NewPreview() Preview {
	cfg := config.Defaults()
	return Preview{
		viewport:    viewport.New(),
		cache:       make(map[renderKey]string),
		maxBytes:    cfg.Preview.MaxBytes,
		wrap:        cfg.Preview.Wrap,
		renderStyle: cfg.Preview.RenderStyle,
		render:      renderDocument,
	}
}

func (p *Preview) SetMaxBytes(maxBytes int64) {
	p.maxBytes = maxBytes
}

func (p *Preview) SetWrap(wrap bool) {
	p.wrap = wrap
	if p.hasCurrent {
		p.current.wrap = wrap
	}
}

func (p *Preview) SetRaw(raw bool) {
	p.raw = raw
	if p.hasCurrent {
		p.current.raw = raw
	}
}

func (p *Preview) SetRenderStyle(style string) {
	if style == "" {
		style = theme.RenderStyleQuiet
	}
	p.renderStyle = style
	if p.hasCurrent {
		p.current.renderStyle = style
	}
}

func (p *Preview) SetDocument(doc notes.Document, width, height int, th theme.Theme) error {
	if err := p.validateDocument(doc); err != nil {
		return err
	}
	key := p.setCurrent(doc, width, height, th)
	rendered, found := p.cache[key]
	if !found || doc.Revision == "" {
		var err error
		rendered, err = p.renderContent(doc, key)
		p.apply(key, rendered)
		return err
	}
	p.apply(key, rendered)
	return nil
}

func (p *Preview) RenderCommand(doc notes.Document, width, height int, th theme.Theme) tea.Cmd {
	key := p.setCurrent(doc, width, height, th)
	return func() tea.Msg {
		if err := p.validateDocument(doc); err != nil {
			return PreviewRenderedMsg{Key: key, Err: err}
		}
		rendered, err := p.renderContent(doc, key)
		return PreviewRenderedMsg{Key: key, Content: rendered, Err: err, Rendered: true}
	}
}

func (p *Preview) Update(msg tea.Msg) (tea.Cmd, error) {
	if rendered, ok := msg.(PreviewRenderedMsg); ok {
		if !p.hasCurrent || rendered.Key != p.current {
			return nil, nil
		}
		if rendered.Rendered {
			p.apply(rendered.Key, rendered.Content)
		}
		return nil, rendered.Err
	}
	var command tea.Cmd
	p.viewport, command = p.viewport.Update(msg)
	return command, nil
}

func (p Preview) View() string {
	return p.viewport.View()
}

func (p *Preview) SetContent(content string, width, height int) {
	ratio := p.viewport.ScrollPercent()
	hadContent := p.viewport.TotalLineCount() > 0
	p.hasCurrent = false
	p.sourceHeadings = nil
	p.toc = nil
	p.tocIndex = -1
	p.viewport.SetWidth(width)
	p.viewport.SetHeight(height)
	p.viewport.SoftWrap = false
	p.viewport.SetContent(content)
	if !hadContent {
		p.viewport.GotoTop()
		return
	}
	maximum := max(0, p.viewport.TotalLineCount()-p.viewport.VisibleLineCount())
	p.viewport.SetYOffset(int(ratio * float64(maximum)))
}

func (p *Preview) Clear() {
	p.hasCurrent = false
	p.sourceHeadings = nil
	p.toc = nil
	p.tocIndex = -1
	p.viewport.SetContent("")
	p.viewport.GotoTop()
}

func (p Preview) validateDocument(doc notes.Document) error {
	if p.maxBytes < 1 || int64(len(doc.Content)) > p.maxBytes {
		return fmt.Errorf("%w: %d bytes exceeds %d byte limit", ErrDocumentTooLarge, len(doc.Content), p.maxBytes)
	}
	return nil
}

func (p *Preview) setCurrent(doc notes.Document, width, height int, th theme.Theme) renderKey {
	marker := ""
	if !p.raw {
		marker = headingMarker(doc.Content)
	}
	key := renderKey{revision: doc.Revision, width: width, theme: th, wrap: p.wrap, raw: p.raw, renderStyle: p.renderStyle, tocMarker: marker}
	if key.revision != p.current.revision {
		p.viewport.GotoTop()
		p.tocIndex = 0
	}
	p.current = key
	p.hasCurrent = true
	p.sourceHeadings = extractHeadings(doc.Content)
	p.toc = nil
	p.viewport.SetWidth(width)
	p.viewport.SetHeight(height)
	p.viewport.SoftWrap = false
	return key
}

func (p Preview) renderContent(doc notes.Document, key renderKey) (string, error) {
	var content string
	var err error
	if key.raw {
		content = strings.ToValidUTF8(string(doc.Content), "\uFFFD")
	} else {
		doc.Content = markHeadings(doc.Content, key.tocMarker)
		content, err = p.render(doc, key.width, false, key.theme, key.renderStyle)
	}
	if key.wrap {
		content = wrapContinuations(content, key.width)
	}
	return content, err
}

func wrapContinuations(content string, width int) string {
	if width <= 2 {
		lines := strings.Split(ansi.Wrap(content, max(1, width), ""), "\n")
		for index := range lines {
			lines[index] = ansi.Truncate(lines[index], max(1, width), "")
		}
		return strings.Join(lines, "\n")
	}
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		parts := strings.Split(ansi.Wrap(line, max(1, width), ""), "\n")
		if len(parts) < 2 {
			continue
		}
		continuations := strings.Split(ansi.Wrap(strings.Join(parts[1:], "\n"), max(1, width-2), ""), "\n")
		for continuation := range continuations {
			if ansi.StringWidth("↪ "+continuations[continuation]) <= width {
				continuations[continuation] = "↪ " + continuations[continuation]
			}
		}
		lines[index] = parts[0] + "\n" + strings.Join(continuations, "\n")
	}
	return strings.Join(lines, "\n")
}

func (p *Preview) apply(key renderKey, rendered string) {
	clear(p.cache)
	if key.revision != "" {
		p.cache[key] = rendered
	}
	if key.raw {
		p.toc = mapRawHeadingLines(p.sourceHeadings, rendered, key.wrap)
	} else {
		p.toc = mapHeadingLines(p.sourceHeadings, rendered, key.tocMarker)
		rendered = strings.ReplaceAll(rendered, key.tocMarker, "")
	}
	if len(p.toc) == 0 {
		p.tocIndex = -1
	} else {
		p.tocIndex = min(max(0, p.tocIndex), len(p.toc)-1)
	}
	p.viewport.SetContent(rendered)
}

func renderDocument(doc notes.Document, width int, wrap bool, th theme.Theme, style string) (string, error) {
	renderer, err := theme.NewMarkdownRenderer(th, width, wrap, style)
	if err != nil {
		return "", err
	}
	return renderer.Render(doc)
}
