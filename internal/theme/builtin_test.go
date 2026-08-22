package theme

import (
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

func TestBuiltinThemesAreCompleteAndSorted(t *testing.T) {
	wantNames := []string{
		"blue-pencil",
		"catppuccin-mocha",
		"dracula",
		"ember-ink",
		"field-notes",
		"gruvbox-dark",
		"highlighter",
		"jotmd",
		"monokai",
		"nord",
		"solarized-dark",
		"solarized-light",
		"tokyo-night",
	}
	if got := Names(); !slices.Equal(got, wantNames) {
		t.Fatalf("Names() = %q, want %q", got, wantNames)
	}

	for _, name := range wantNames {
		t.Run(name, func(t *testing.T) {
			th, err := Builtin(name)
			if err != nil {
				t.Fatal(err)
			}
			if th.Name != name {
				t.Errorf("Builtin(%q).Name = %q", name, th.Name)
			}
			if styles.Registry[th.SyntaxStyle] == nil {
				t.Errorf("Builtin(%q).SyntaxStyle = %q, want a Glamour style", name, th.SyntaxStyle)
			}
			palette := reflect.ValueOf(th.Palette)
			for index := range palette.NumField() {
				field := palette.Type().Field(index)
				if field.Name == "Background" && palette.Field(index).String() == "" {
					continue
				}
				if color := palette.Field(index).String(); !chroma.ParseColour(color).IsSet() {
					t.Errorf("Builtin(%q).Palette.%s = %q, want a color", name, field.Name, color)
				}
			}
			if th.Palette.Background != "" {
				assertContrast(t, th.Palette.Foreground, th.Palette.Background)
				assertContrast(t, th.Palette.SelectionBackground, th.Palette.Background)
				assertContrast(t, th.Palette.Background, th.Palette.Status)
			}
			assertContrast(t, th.Palette.SelectionForeground, th.Palette.SelectionBackground)
		})
	}
}

func TestBuiltinSyntaxStylesFollowTheme(t *testing.T) {
	want := map[string]string{
		"jotmd": "rose-pine", "catppuccin-mocha": "catppuccin-mocha",
		"field-notes": "rose-pine", "blue-pencil": "github-dark", "highlighter": "gruvbox", "ember-ink": "rose-pine-moon",
		"dracula": "dracula", "nord": "nord", "gruvbox-dark": "gruvbox", "tokyo-night": "tokyonight-night",
		"solarized-dark": "solarized-dark", "solarized-light": "solarized-light", "monokai": "monokai",
	}
	for name, style := range want {
		th, err := Builtin(name)
		if err != nil {
			t.Fatal(err)
		}
		if th.SyntaxStyle != style {
			t.Errorf("Builtin(%q).SyntaxStyle = %q, want %q", name, th.SyntaxStyle, style)
		}
	}
}

func TestApplyOverridesThemeColors(t *testing.T) {
	base, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	th, err := Apply(base, map[string]string{"accent": "#FFFFFF"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if th.Palette.Accent != "#FFFFFF" {
		t.Errorf("Apply() Accent = %q, want override", th.Palette.Accent)
	}
}

func TestApplyRejectsUnknownAndInvalidColors(t *testing.T) {
	base, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	for _, overrides := range []map[string]string{{"unknown": "#FFFFFF"}, {"accent": "not-a-color"}} {
		if _, err := Apply(base, overrides, false); err == nil {
			t.Errorf("Apply() error = nil for %#v", overrides)
		}
	}
}

func TestApplyNoColorClearsSemanticColors(t *testing.T) {
	base, err := Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	th, err := Apply(base, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	palette := reflect.ValueOf(th.Palette)
	for index := range palette.NumField() {
		if got := palette.Field(index).String(); got != "" {
			t.Errorf("Apply(no color).Palette.%s = %q, want empty", palette.Type().Field(index).Name, got)
		}
	}
}

func assertContrast(t *testing.T, foreground, background string) {
	t.Helper()
	if contrast(chroma.ParseColour(foreground), chroma.ParseColour(background)) < 3 {
		t.Errorf("contrast(%q, %q) is too low", foreground, background)
	}
}

func contrast(first, second chroma.Colour) float64 {
	firstLuminance := luminance(first)
	secondLuminance := luminance(second)
	if firstLuminance < secondLuminance {
		firstLuminance, secondLuminance = secondLuminance, firstLuminance
	}
	return (firstLuminance + 0.05) / (secondLuminance + 0.05)
}

func luminance(color chroma.Colour) float64 {
	channel := func(value uint8) float64 {
		channel := float64(value) / 255
		if channel <= 0.03928 {
			return channel / 12.92
		}
		return math.Pow((channel+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(color.Red()) + 0.7152*channel(color.Green()) + 0.0722*channel(color.Blue())
}
