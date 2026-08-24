package theme

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/notes"
)

type Renderer struct {
	render   func(string) (string, error)
	decorate func(string) string
}

const (
	codeBlockStart = '\ue000'
	codeBlockEnd   = '\ue001'
)

var sgrPattern = regexp.MustCompile(`\x1b\[[0-9:;]*m`)

const (
	RenderStyleQuiet      = "quiet"
	RenderStyleSurface    = "surface"
	RenderStyleStructural = "structural"
)

func RenderStyles() []string {
	return []string{RenderStyleQuiet, RenderStyleSurface, RenderStyleStructural}
}

func StripStyles(value string) string {
	return sgrPattern.ReplaceAllString(value, "")
}

func NewMarkdownRenderer(theme Theme, width int, wrap bool, style string) (*Renderer, error) {
	if width < 1 {
		return nil, fmt.Errorf("markdown width must be positive")
	}
	if style != RenderStyleQuiet && style != RenderStyleSurface && style != RenderStyleStructural {
		return nil, fmt.Errorf("unknown markdown render style %q", style)
	}
	wordWrap := 0
	if wrap {
		wordWrap = width
	}
	styles := markdownStyles(theme, style)
	return &Renderer{render: func(content string) (string, error) {
		start, end, err := codeBlockMarkers(content)
		if err != nil {
			return "", err
		}
		renderStyles := styles
		renderStyles.CodeBlock.BlockPrefix = start
		renderStyles.CodeBlock.BlockSuffix = end
		renderer, err := glamour.NewTermRenderer(
			glamour.WithStyles(renderStyles),
			glamour.WithWordWrap(wordWrap),
			glamour.WithPreservedNewLines(),
		)
		if err != nil {
			return "", fmt.Errorf("create markdown renderer: %w", err)
		}
		rendered, err := renderer.Render(content)
		if err != nil {
			return "", err
		}
		return decorateCodeBlocks(rendered, theme, width, start, end), nil
	}}, nil
}

func (r *Renderer) Render(doc notes.Document) (string, error) {
	content := string(bytes.ToValidUTF8(doc.Content, []byte("\uFFFD")))
	rendered, err := r.render(content)
	if err != nil {
		return content, err
	}
	if r.decorate != nil {
		rendered = r.decorate(rendered)
	}
	return removeBackgroundColors(rendered), nil
}

func codeBlockMarkers(content string) (string, string, error) {
	used := make(map[rune]struct{})
	for _, char := range html.UnescapeString(content) {
		used[char] = struct{}{}
	}
	markers := make([]rune, 0, 2)
	for _, bounds := range [][2]rune{{codeBlockStart, 0xF8FF}, {0xF0000, 0xFFFFD}, {0x100000, 0x10FFFD}} {
		for marker := bounds[0]; marker <= bounds[1] && len(markers) < 2; marker++ {
			if _, found := used[marker]; !found {
				markers = append(markers, marker)
			}
		}
		if len(markers) == 2 {
			return string(markers[0]), string(markers[1]), nil
		}
	}
	return "", "", fmt.Errorf("markdown contains every private-use delimiter")
}

func decorateCodeBlocks(rendered string, theme Theme, width int, start, end string) string {
	gutter := lipgloss.NewStyle()
	if theme.Palette.Accent != "" {
		gutter = gutter.Foreground(lipgloss.Color(theme.Palette.Accent))
	}

	var output strings.Builder
	inCode := false
	for _, original := range strings.SplitAfter(rendered, "\n") {
		line := original
		if before, after, found := strings.Cut(line, start); found {
			line = before + after
			inCode = true
		}
		if before, after, found := strings.Cut(line, end); found {
			if inCode && strings.TrimSpace(xansi.Strip(before)) != "" {
				output.WriteString(decorateCodeLine(before, gutter, width))
			}
			output.WriteString(after)
			inCode = false
			continue
		}
		if inCode {
			output.WriteString(decorateCodeLine(line, gutter, width))
		} else {
			output.WriteString(line)
		}
	}
	return output.String()
}

func decorateCodeLine(line string, gutter lipgloss.Style, width int) string {
	newline := ""
	if strings.HasSuffix(line, "\n") {
		line = strings.TrimSuffix(line, "\n")
		newline = "\n"
	}
	line = removeBackgroundColors(line)
	if width < 3 {
		return line + newline
	}
	plain := xansi.Strip(line)
	overflow := max(0, xansi.StringWidth(line)+2-width)
	padding := len(plain) - len(strings.TrimRight(plain, " "))
	if trim := min(overflow, padding); trim > 0 {
		line = xansi.Truncate(line, xansi.StringWidth(line)-trim, "")
	}
	line = lipgloss.Wrap(line, width-2, "")
	return gutter.Render("┃ ") + strings.ReplaceAll(line, "\n", "\n"+gutter.Render("┃ ")) + newline
}

func removeBackgroundColors(s string) string {
	return sgrPattern.ReplaceAllStringFunc(s, func(sequence string) string {
		params := strings.Split(sequence[2:len(sequence)-1], ";")
		kept := make([]string, 0, len(params))
		for i := 0; i < len(params); i++ {
			parameter := params[i]
			valueString, _, _ := strings.Cut(parameter, ":")
			value, err := strconv.Atoi(valueString)
			if err != nil {
				kept = append(kept, parameter)
				continue
			}
			if strings.Contains(parameter, ":") {
				if value == 48 || value >= 40 && value <= 49 || value >= 100 && value <= 107 {
					continue
				}
				kept = append(kept, parameter)
				continue
			}
			if value == 38 || value == 58 {
				end := i + 1
				if end < len(params) {
					switch params[end] {
					case "5":
						end = min(i+3, len(params))
					case "2":
						end = min(i+5, len(params))
					}
				}
				kept = append(kept, params[i:end]...)
				i = end - 1
				continue
			}
			if value == 48 {
				if i+1 < len(params) && params[i+1] == "5" {
					i += min(2, len(params)-i-1)
				} else if i+1 < len(params) && params[i+1] == "2" {
					i += min(4, len(params)-i-1)
				}
				continue
			}
			if value >= 40 && value <= 49 || value >= 100 && value <= 107 {
				continue
			}
			kept = append(kept, params[i])
		}
		if len(kept) == len(params) {
			return sequence
		}
		if len(kept) == 0 {
			return ""
		}
		return "\x1b[" + strings.Join(kept, ";") + "m"
	})
}

func markdownStyles(theme Theme, renderStyle string) ansi.StyleConfig {
	p := theme.Palette
	syntaxStyle := theme.SyntaxStyle
	if p.Foreground == "" {
		syntaxStyle = ""
	}
	styles := ansi.StyleConfig{
		Document:  ansi.StyleBlock{StylePrimitive: primitive(p.Foreground, "")},
		Paragraph: ansi.StyleBlock{StylePrimitive: primitive(p.Foreground, "")},
		BlockQuote: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
			Prefix: "│ ", Color: color(p.Muted),
		}},
		List:     ansi.StyleList{StyleBlock: ansi.StyleBlock{StylePrimitive: primitive(p.Foreground, "")}},
		Item:     ansi.StylePrimitive{BlockPrefix: "• ", Color: color(p.Directory)},
		Task:     ansi.StyleTask{StylePrimitive: ansi.StylePrimitive{Color: color(p.Accent)}, Ticked: "[✓] ", Unticked: "[ ] "},
		Heading:  ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: color(p.Heading), Bold: boolPointer(true)}},
		H1:       ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: color(p.Heading), Bold: boolPointer(true)}},
		H2:       ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: color(p.Heading), Bold: boolPointer(true)}},
		H3:       ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: color(p.Heading), Bold: boolPointer(true)}},
		H4:       ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: color(p.Heading), Bold: boolPointer(true)}},
		H5:       ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: color(p.Heading), Bold: boolPointer(true)}},
		H6:       ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Color: color(p.Heading), Bold: boolPointer(true)}},
		Link:     ansi.StylePrimitive{Color: color(p.Link), Underline: boolPointer(true)},
		LinkText: ansi.StylePrimitive{Color: color(p.Link)},
		Code:     ansi.StyleBlock{StylePrimitive: primitive(p.Code, "")},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{StylePrimitive: primitive(p.Code, "")},
			Theme:      syntaxStyle,
		},
		HorizontalRule: ansi.StylePrimitive{Color: color(p.Border), Format: "\n────\n"},
	}
	switch renderStyle {
	case RenderStyleSurface:
		styles.BlockQuote.BackgroundColor = color(p.Border)
		styles.Code.BackgroundColor = color(p.Border)
	case RenderStyleStructural:
		styles.H1.Prefix = "◆ "
		styles.H2.Prefix = "── "
		styles.BlockQuote.Prefix = "NOTE │ "
	}
	return styles
}

func primitive(foreground, background string) ansi.StylePrimitive {
	return ansi.StylePrimitive{Color: color(foreground), BackgroundColor: color(background)}
}

func color(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func boolPointer(value bool) *bool { return &value }
