package theme

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/notes"
)

func TestBuiltinJotMDHasEverySemanticColor(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "jotmd" {
		t.Errorf("Builtin(\"jotmd\").Name = %q, want jotmd", th.Name)
	}
	if th.SyntaxStyle == "" {
		t.Error("Builtin(\"jotmd\").SyntaxStyle is empty")
	}
	palette := reflect.ValueOf(th.Palette)
	for index := range palette.NumField() {
		field := palette.Type().Field(index)
		if field.Name == "Background" {
			continue
		}
		if palette.Field(index).String() == "" {
			t.Errorf("Builtin(\"jotmd\").Palette.%s is empty", field.Name)
		}
	}
	if _, err := Builtin("unknown"); err == nil {
		t.Error("Builtin(\"unknown\") error = nil, want rejection")
	}
}

func TestMarkdownRendererRendersRequiredConstructs(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewMarkdownRenderer(th, 80, true, RenderStyleQuiet)
	if err != nil {
		t.Fatal(err)
	}
	doc := notes.Document{Content: []byte("# Heading\n\n- item\n- [x] done\n- [ ] todo\n\n```go\ncode()\n```\n\n`inline` [link](https://example.com)\n\n> quote\n\n---\n")}

	rendered, err := renderer.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Heading", "item", "done", "code", "inline", "link", "quote"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("Render() = %q, missing %q", rendered, want)
		}
	}
	plain := xansi.Strip(rendered)
	for _, want := range []string{"[✓] done", "[ ] todo", "─"} {
		if !strings.Contains(plain, want) {
			t.Errorf("Render() = %q, missing Markdown marker %q", rendered, want)
		}
	}
}

func TestMarkdownRendererPreservesSourceLineBreaks(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewMarkdownRenderer(th, 80, false, RenderStyleQuiet)
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := renderer.Render(notes.Document{Content: []byte("first line\nsecond line\n")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(xansi.Strip(rendered), "first line\nsecond line") {
		t.Fatalf("Render() = %q, want source lines kept separate", rendered)
	}
}

func TestMarkdownRendererEmitsTerminalHyperlinks(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewMarkdownRenderer(th, 80, false, RenderStyleQuiet)
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := renderer.Render(notes.Document{Content: []byte("[docs](https://example.com/docs)\n")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "\x1b]8;") || !strings.Contains(rendered, "https://example.com/docs") {
		t.Fatalf("Render() = %q, want OSC-8 hyperlink target", rendered)
	}
}

func TestMarkdownRendererNoColorDoesNotEmitANSI(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	th, err = Apply(th, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewMarkdownRenderer(th, 80, true, RenderStyleQuiet)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderer.Render(notes.Document{Content: []byte("plain text\n\n```go\nfunc main() {}\n```\n")})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "\x1b[") {
		t.Errorf("Render() = %q, want monochrome output", rendered)
	}
	if !strings.Contains(rendered, "┃ ") {
		t.Errorf("Render() = %q, want structural code gutter", rendered)
	}
}

func TestMarkdownRendererStylesCodeBlocksFromActiveTheme(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			th, err := Builtin(name)
			if err != nil {
				t.Fatal(err)
			}
			renderer, err := NewMarkdownRenderer(th, 80, true, RenderStyleQuiet)
			if err != nil {
				t.Fatal(err)
			}
			rendered, err := renderer.Render(notes.Document{Content: []byte("```go\nfunc main() {}\nreturn\n```\n\n    indented\n")})
			if err != nil {
				t.Fatal(err)
			}

			if got := strings.Count(xansi.Strip(rendered), "┃ "); got != 3 {
				t.Fatalf("Render() has %d code gutters, want 3: %q", got, rendered)
			}
			if strings.ContainsRune(rendered, codeBlockStart) || strings.ContainsRune(rendered, codeBlockEnd) {
				t.Fatalf("Render() leaked code block markers: %q", rendered)
			}
			for _, line := range strings.Split(rendered, "\n") {
				if strings.Contains(xansi.Strip(line), "┃ ") && xansi.StringWidth(line) > 80 {
					t.Fatalf("rendered code line width = %d, want at most 80: %q", xansi.StringWidth(line), line)
				}
			}
			if removeBackgroundColors(rendered) != rendered {
				t.Fatalf("Render() paints a code background: %q", rendered)
			}
			gutter := lipgloss.NewStyle().
				Foreground(lipgloss.Color(th.Palette.Accent)).
				Render("┃")
			gutterPrefix := strings.SplitN(gutter, "┃", 2)[0]
			if gutterPrefix == "" || !strings.Contains(rendered, gutterPrefix) {
				t.Fatalf("Render() = %q, missing accent gutter %q", rendered, gutterPrefix)
			}
			for _, want := range []string{"func main() {}", "return", "indented"} {
				if !strings.Contains(xansi.Strip(rendered), want) {
					t.Fatalf("Render() = %q, missing code %q", rendered, want)
				}
			}
			if !strings.Contains(rendered, "\x1b[38;") {
				t.Fatalf("Render() = %q, missing syntax foreground colors", rendered)
			}
		})
	}
}

func TestMarkdownRendererWrapsLongCodeInsideGutter(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewMarkdownRenderer(th, 80, true, RenderStyleQuiet)
	if err != nil {
		t.Fatal(err)
	}
	for _, length := range []int{79, 80, 159} {
		t.Run(strconv.Itoa(length), func(t *testing.T) {
			code := strings.Repeat("x", length)
			rendered, err := renderer.Render(notes.Document{Content: []byte("```text\n" + code + "\n```\n")})
			if err != nil {
				t.Fatal(err)
			}

			if strings.ContainsRune(rendered, codeBlockStart) || strings.ContainsRune(rendered, codeBlockEnd) {
				t.Fatalf("Render() leaked code block markers: %q", rendered)
			}
			var got strings.Builder
			for _, line := range strings.Split(rendered, "\n") {
				plain := xansi.Strip(line)
				if strings.TrimSpace(plain) == "" {
					continue
				}
				if !strings.HasPrefix(plain, "┃ ") {
					t.Fatalf("rendered code continuation has no gutter: %q", line)
				}
				if width := xansi.StringWidth(line); width > 80 {
					t.Fatalf("rendered code line width = %d, want at most 80: %q", width, line)
				}
				_, afterGutter, found := strings.Cut(line, "┃ ")
				codeStart := strings.Index(afterGutter, "x")
				if !found || codeStart < 0 || !strings.Contains(afterGutter[:codeStart], "\x1b[38;") {
					t.Fatalf("rendered code continuation has no active foreground: %q", line)
				}
				got.WriteString(strings.TrimRight(strings.TrimPrefix(plain, "┃ "), " "))
			}
			if got.String() != code {
				t.Fatalf("rendered code = %q, want %q: %q", got.String(), code, rendered)
			}
		})
	}
}

func TestMarkdownRendererPreservesPrivateUseCharacters(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewMarkdownRenderer(th, 80, true, RenderStyleQuiet)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderer.Render(notes.Document{Content: []byte("before \ue000 and &#xE002; after\n\n```text\nx\ue001y\n```\n")})
	if err != nil {
		t.Fatal(err)
	}
	plain := xansi.Strip(rendered)
	for _, want := range []string{"before \ue000 and \ue002 after", "x\ue001y"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("Render() = %q, missing literal %q", rendered, want)
		}
	}
	if got := strings.Count(plain, "┃ "); got != 1 {
		t.Fatalf("Render() has %d code gutters, want 1: %q", got, rendered)
	}
}

func TestMarkdownRendererOmitsGutterWhenItCannotFit(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := NewMarkdownRenderer(th, 2, true, RenderStyleQuiet)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderer.Render(notes.Document{Content: []byte("```text\nx\n```\n")})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(xansi.Strip(rendered), "┃ ") {
		t.Fatalf("Render() = %q, want no gutter below three columns", rendered)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if width := xansi.StringWidth(line); width > 2 {
			t.Fatalf("rendered line width = %d, want at most 2: %q", width, line)
		}
	}
}

func TestRemoveBackgroundColors(t *testing.T) {
	for input, want := range map[string]string{
		"\x1b[44mcode\x1b[0m":                    "code\x1b[0m",
		"\x1b[48;5;235mcode\x1b[0m":              "code\x1b[0m",
		"\x1b[38;2;1;2;3;48;2;4;5;6mcode\x1b[0m": "\x1b[38;2;1;2;3mcode\x1b[0m",
		"\x1b[38;2;40;41;42mcode\x1b[0m":         "\x1b[38;2;40;41;42mcode\x1b[0m",
		"\x1b[38;5;44mcode\x1b[0m":               "\x1b[38;5;44mcode\x1b[0m",
		"\x1b[1;104;38;5;2mcode\x1b[0m":          "\x1b[1;38;5;2mcode\x1b[0m",
	} {
		if got := removeBackgroundColors(input); got != want {
			t.Errorf("removeBackgroundColors(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMarkdownRendererRemovesColonFormBackgroundColors(t *testing.T) {
	renderer := Renderer{render: func(string) (string, error) {
		return "\x1b[38:2::1:2:3;48:2::4:5:6m┃ code\x1b[0m", nil
	}}

	rendered, err := renderer.Render(notes.Document{Content: []byte("code")})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, ";48:") {
		t.Fatalf("Render() retained a colon-form background: %q", rendered)
	}
	if !strings.Contains(rendered, "\x1b[38:2::1:2:3m┃ code") {
		t.Fatalf("Render() = %q, want foreground and gutter preserved", rendered)
	}
}

func TestMarkdownRendererHonorsWrapSetting(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	doc := notes.Document{Content: []byte("one two three four five six\n")}

	wrapped, err := NewMarkdownRenderer(th, 10, true, RenderStyleQuiet)
	if err != nil {
		t.Fatal(err)
	}
	wrappedContent, err := wrapped.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	unwrapped, err := NewMarkdownRenderer(th, 10, false, RenderStyleQuiet)
	if err != nil {
		t.Fatal(err)
	}
	unwrappedContent, err := unwrapped.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(wrappedContent, "\n") <= strings.Count(unwrappedContent, "\n") {
		t.Error("enabled wrapping did not add rendered line breaks")
	}
}

func TestMarkdownRenderStyles(t *testing.T) {
	th, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	quiet := markdownStyles(th, RenderStyleQuiet)
	if quiet.Document.BackgroundColor != nil || quiet.Paragraph.BackgroundColor != nil || quiet.Code.BackgroundColor != nil || quiet.CodeBlock.BackgroundColor != nil {
		t.Fatal("quiet style paints a Markdown background")
	}
	surface := markdownStyles(th, RenderStyleSurface)
	if surface.Code.BackgroundColor == nil || *surface.Code.BackgroundColor != th.Palette.Border || surface.CodeBlock.BackgroundColor != nil {
		t.Fatal("surface style should paint inline code, but not code blocks")
	}
	structural := markdownStyles(th, RenderStyleStructural)
	if structural.H1.Prefix == "" || structural.H2.Prefix == "" {
		t.Fatal("structural style does not distinguish heading levels")
	}
	if got := strings.Join(RenderStyles(), ","); got != "quiet,surface,structural" {
		t.Fatalf("RenderStyles() = %q", got)
	}
	renderer, err := NewMarkdownRenderer(th, 80, true, RenderStyleQuiet)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := renderer.Render(notes.Document{Content: []byte("paragraph `inline`\n")})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "\x1b[48;") {
		t.Fatalf("quiet renderer paints a terminal background: %q", rendered)
	}
}

func TestMarkdownRendererReturnsSafeFallbackOnRenderError(t *testing.T) {
	wantErr := errors.New("render failed")
	doc := notes.Document{Content: []byte{'b', 'a', 'd', 0xff}}
	renderer := Renderer{render: func(string) (string, error) { return "", wantErr }}

	rendered, err := renderer.Render(doc)
	if !errors.Is(err, wantErr) {
		t.Errorf("Render() error = %v, want %v", err, wantErr)
	}
	if rendered != "bad\uFFFD" {
		t.Errorf("Render() fallback = %q, want replacement-safe source", rendered)
	}
	if string(doc.Content) != "bad\xff" {
		t.Errorf("Render() changed Document.Content = %v", doc.Content)
	}
}

func TestMarkdownRendererReplacesInvalidUTF8OnlyInRenderInput(t *testing.T) {
	doc := notes.Document{Content: []byte{'b', 'a', 'd', 0xff}}
	renderer := Renderer{render: func(content string) (string, error) { return content, nil }}

	rendered, err := renderer.Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if rendered != "bad\uFFFD" {
		t.Errorf("Render() = %q, want replacement-safe source", rendered)
	}
	if string(doc.Content) != "bad\xff" {
		t.Errorf("Render() changed Document.Content = %v", doc.Content)
	}
}
