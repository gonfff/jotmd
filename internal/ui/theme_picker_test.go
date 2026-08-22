package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/notes"
	"github.com/gonfff/jotmd/internal/theme"
)

func TestModelThemePickerUsesRegisteredSelectActionAndConfiguredBindings(t *testing.T) {
	base := jotmdTheme(t)
	alternate := base
	alternate.Name = "alternate"
	model := NewModelWithOptions(testStore(t), config.Defaults(), base, config.Keymap{}, []theme.Theme{base, alternate})

	if !hasAction(ActionRegistry(), "theme.select") {
		t.Fatal("theme.select is not registered")
	}
	model = updateModel(t, model, key("t"))
	if model.mode == Browse {
		t.Fatal("theme picker did not enter an overlay mode")
	}
	model = updateModel(t, model, key("j"))
	if model.theme.Name != "jotmd" {
		t.Fatalf("printable key changed theme to %q", model.theme.Name)
	}
	if !strings.Contains(ansi.Strip(model.statusView(80, ContextTheme)), "Theme: j") {
		t.Fatalf("picker status = %q, want query j", model.statusView(80, ContextTheme))
	}
	model = updateModel(t, model, key("down"))
	if model.theme.Name != "jotmd" {
		t.Fatalf("filtered down changed theme to %q", model.theme.Name)
	}
	model = updateModel(t, model, key("backspace"))
	model = updateModel(t, model, key("down"))
	if model.theme.Name != "alternate" {
		t.Fatalf("down left theme at %q", model.theme.Name)
	}
	model = updateModel(t, model, key("ctrl+p"))
	if model.theme.Name != "jotmd" {
		t.Fatalf("ctrl+p left theme at %q", model.theme.Name)
	}
	model = updateModel(t, model, key("down"))
	view := model.themePickerView(80, 10)
	for _, redundant := range []string{"Themes", "up", "down", "enter", "esc"} {
		if strings.Contains(ansi.Strip(view), redundant) {
			t.Errorf("theme picker retains %q: %q", redundant, view)
		}
	}
	model = updateModel(t, model, key("enter"))
	if model.mode != Browse || model.cfg.Theme != "jotmd" {
		t.Fatalf("configured apply left picker = (%v, %q)", model.mode, model.cfg.Theme)
	}
	model = updateModel(t, model, key("t"))
	model = updateModel(t, model, key("up"))
	model = updateModel(t, model, key("esc"))
	if model.mode != Browse || model.theme.Name != "alternate" {
		t.Fatalf("configured close left picker = (%v, %q)", model.mode, model.theme.Name)
	}
}

func TestModelThemePickerPreviewsAndRestores(t *testing.T) {
	base := jotmdTheme(t)
	alternate := base
	alternate.Name = "alternate"
	alternate.Palette.Accent = "#FFFFFF"
	model := NewModelWithOptions(testStore(t), config.Defaults(), base, config.Keymap{}, []theme.Theme{base, alternate})
	model = updateModel(t, model, key("t"))
	if model.mode == Browse {
		t.Fatal("theme picker did not open")
	}
	model = updateModel(t, model, key("down"))
	if model.theme.Name != "alternate" {
		t.Fatalf("preview theme = %q, want alternate", model.theme.Name)
	}
	model = updateModel(t, model, key("esc"))
	if model.theme != base || model.mode != Browse {
		t.Fatalf("escape did not restore original theme: %#v", model.theme)
	}
}

func TestModelThemePickerAcceptsThemeWithoutChangingConfiguration(t *testing.T) {
	base := jotmdTheme(t)
	alternate := base
	alternate.Name = "alternate"
	model := NewModelWithOptions(testStore(t), config.Defaults(), base, config.Keymap{}, []theme.Theme{base, alternate})
	model = updateModel(t, model, key("t"))
	model = updateModel(t, model, key("down"))
	model = updateModel(t, model, key("enter"))
	if model.theme.Name != "alternate" || model.cfg.Theme != "jotmd" || model.mode != Browse {
		t.Fatalf("accepted picker = (%q, %q, %v), want (alternate, jotmd, Browse)", model.theme.Name, model.cfg.Theme, model.mode)
	}
}

func TestThemePickerFiltersInBottomOverlayAndNoMatchesDoNotApply(t *testing.T) {
	base := jotmdTheme(t)
	match := base
	match.Name = "dracula"
	other := base
	other.Name = "nord"
	cfg := config.Defaults()
	cfg.StatusBar = false
	model := NewModelWithOptions(testStore(t), cfg, base, config.Keymap{}, []theme.Theme{base, match, other})
	model.width, model.height = 80, 16
	model = updateModel(t, model, key("t"))
	model = updateModel(t, model, key("d"))
	model = updateModel(t, model, key("r"))

	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Theme: dr", "> dracula"} {
		if !strings.Contains(view, want) {
			t.Fatalf("picker view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(model.themePickerView(80, 10), "nord") {
		t.Fatalf("filtered picker includes non-match: %q", model.themePickerView(80, 10))
	}
	if !model.statusBarVisible() {
		t.Fatal("theme picker did not force status visibility")
	}
	lines := strings.Split(view, "\n")
	if popupRow, statusRow := -1, len(lines)-1; !strings.Contains(lines[statusRow], "Theme: dr") {
		t.Fatalf("picker status is not bottom row: %q", lines[statusRow])
	} else {
		for index, line := range lines {
			if strings.Contains(line, "dracula") {
				popupRow = index
			}
		}
		if popupRow < len(lines)/2 {
			t.Fatalf("picker popup is not bottom-aligned:\n%s", view)
		}
	}

	model = updateModel(t, model, key("backspace"))
	model = updateModel(t, model, key("backspace"))
	model = updateModel(t, model, key("x"))
	if !strings.Contains(ansi.Strip(model.statusView(80, ContextTheme)), "Theme: x") || !strings.Contains(ansi.Strip(model.themePickerView(80, 10)), "No matches.") {
		t.Fatalf("no-match picker = (%q, %q)", model.statusView(80, ContextTheme), model.themePickerView(80, 10))
	}
	model = updateModel(t, model, key("enter"))
	if model.mode != ThemePicker || model.theme != base {
		t.Fatalf("enter on no matches = (mode=%v, theme=%q), want open/base", model.mode, model.theme.Name)
	}
}

func TestPickersSyncTreeViewportWhenTheyForceStatusBar(t *testing.T) {
	for _, test := range []struct {
		picker string
		close  string
	}{
		{picker: "t", close: "esc"},
		{picker: "t", close: "enter"},
		{picker: "s", close: "esc"},
		{picker: "s", close: "enter"},
	} {
		t.Run(test.picker+"/"+test.close, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.StatusBar = false
			model := newModel(testStore(t), cfg, jotmdTheme(t))
			model = updateModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
			if model.tree.height != paneHeight(model.height, false) {
				t.Fatalf("initial tree height = %d, want %d", model.tree.height, paneHeight(model.height, false))
			}

			model = updateModel(t, model, key(test.picker))
			if model.tree.height != paneHeight(model.height, model.statusBarVisible()) {
				t.Fatalf("open tree height = %d, want %d", model.tree.height, paneHeight(model.height, model.statusBarVisible()))
			}

			model = updateModel(t, model, key(test.close))
			if model.tree.height != paneHeight(model.height, model.statusBarVisible()) {
				t.Fatalf("close tree height = %d, want %d", model.tree.height, paneHeight(model.height, model.statusBarVisible()))
			}
		})
	}
}

func TestModelThemePickerEscapeRestoresReloadedEffectiveTheme(t *testing.T) {
	base := jotmdTheme(t)
	preview := builtinTheme(t, "dracula")
	reloaded := builtinTheme(t, "nord")
	model := NewModelWithOptions(testStore(t), config.Defaults(), base, config.Keymap{}, []theme.Theme{base, preview, reloaded})
	model = updateModel(t, model, key("t"))
	model = updateModel(t, model, key("down"))
	if model.theme != preview {
		t.Fatalf("picker preview = %q, want dracula", model.theme.Name)
	}

	cfg := config.Defaults()
	cfg.Theme = "nord"
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{Config: cfg, theme: reloaded}})
	selected := model.themes[model.themePicker.filtered[model.themePicker.selected]]
	if model.mode != ThemePicker || model.themePicker.original != reloaded || selected != reloaded {
		t.Fatalf("picker after reload = (mode=%v, original=%q, selected=%q), want reloaded nord", model.mode, model.themePicker.original.Name, selected.Name)
	}

	model = updateModel(t, model, key("esc"))
	if model.mode != Browse || model.theme != reloaded {
		t.Fatalf("picker escape after reload = (mode=%v, theme=%q), want Browse/nord", model.mode, model.theme.Name)
	}
}

func TestThemePickerReloadKeepsQuery(t *testing.T) {
	base := jotmdTheme(t)
	match := builtinTheme(t, "dracula")
	reloaded := builtinTheme(t, "nord")
	model := NewModelWithOptions(testStore(t), config.Defaults(), base, config.Keymap{}, []theme.Theme{base, match, reloaded})
	model = updateModel(t, model, key("t"))
	model = updateModel(t, model, key("d"))
	model = updateModel(t, model, key("r"))
	cfg := config.Defaults()
	cfg.Theme = "nord"
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{Config: cfg, theme: reloaded}})
	if !strings.Contains(ansi.Strip(model.statusView(80, ContextTheme)), "Theme: dr") || !strings.Contains(ansi.Strip(model.themePickerView(80, 10)), "dracula") {
		t.Fatalf("picker reload lost query: status=%q view=%q", model.statusView(80, ContextTheme), model.themePickerView(80, 10))
	}
	if model.theme != match {
		t.Fatalf("picker reload preview = %q, want filtered dracula", model.theme.Name)
	}
}

func TestModelThemePickerRerendersPreviewForSelectionAndResize(t *testing.T) {
	base := jotmdTheme(t)
	alternate := base
	alternate.Name = "alternate"
	model := NewModelWithOptions(testStore(t), config.Defaults(), base, config.Keymap{}, []theme.Theme{base, alternate})
	model = updateModel(t, model, run(model.Init()))
	model, command := updateModelCommand(t, model, key("j"))
	model = updateModel(t, model, run(command))
	model.preview.render = func(_ notes.Document, _ int, _ bool, th theme.Theme, _ string) (string, error) {
		return th.Name, nil
	}
	model, command = updateModelCommand(t, model, tea.WindowSizeMsg{Width: 80, Height: 12})
	model = updateModel(t, model, run(command))
	if model.preview.viewport.GetContent() != "jotmd" {
		t.Fatalf("initial preview = %q, want jotmd", model.preview.viewport.GetContent())
	}

	model = updateModel(t, model, key("t"))
	model, command = updateModelCommand(t, model, key("down"))
	model = updateModel(t, model, run(command))
	if model.preview.current.theme != alternate || model.preview.viewport.GetContent() != "alternate" || len(model.preview.cache) != 1 {
		t.Fatalf("theme preview = (%#v, %q, %d), want alternate theme, content, and one cache entry", model.preview.current.theme, model.preview.viewport.GetContent(), len(model.preview.cache))
	}

	model, command = updateModelCommand(t, model, tea.WindowSizeMsg{Width: 120, Height: 20})
	model = updateModel(t, model, run(command))
	if model.preview.current.width != previewWidth(120, model.cfg.TreeWidth) || model.preview.current.theme != alternate || model.preview.viewport.GetContent() != "alternate" {
		t.Fatalf("resized preview = (%d, %#v, %q)", model.preview.current.width, model.preview.current.theme, model.preview.viewport.GetContent())
	}
}

func TestModelThemePickerSkipsInvalidCatalogThemes(t *testing.T) {
	base := jotmdTheme(t)
	invalid := base
	invalid.Name = "invalid"
	invalid.Palette.Accent = ""
	model := NewModelWithOptions(testStore(t), config.Defaults(), base, config.Keymap{}, []theme.Theme{base, invalid})
	if len(model.themes) != 1 || model.themes[0] != base {
		t.Fatalf("picker themes = %#v, want only %#v", model.themes, base)
	}
}

func TestModelThemePickerPreservesNoColorTheme(t *testing.T) {
	base := jotmdTheme(t)
	base.Palette = theme.Palette{}
	alternate := jotmdTheme(t)
	alternate.Name = "alternate"
	cfg := config.Defaults()
	cfg.NoColor = true
	model := NewModelWithOptions(testStore(t), cfg, base, config.Keymap{}, []theme.Theme{jotmdTheme(t), alternate})

	model = updateModel(t, model, key("t"))
	model = updateModel(t, model, key("down"))
	if model.theme.Palette != (theme.Palette{}) {
		t.Fatalf("selected no-color theme palette = %#v, want empty", model.theme.Palette)
	}
}

func TestThemePickerKeepsSelectedThemeAndFooterVisible(t *testing.T) {
	base := jotmdTheme(t)
	themes := make([]theme.Theme, 0, len(theme.Names())+1)
	for _, name := range theme.Names() {
		candidate, err := theme.Builtin(name)
		if err != nil {
			t.Fatal(err)
		}
		themes = append(themes, candidate)
	}
	custom := base
	custom.Name = "zz-custom"
	themes = append(themes, custom)
	model := NewModelWithOptions(testStore(t), config.Defaults(), base, config.Keymap{}, themes)
	model.openThemePicker()
	for range len(model.themes) - 1 {
		model.updateThemePicker(ActionThemeDown)
	}

	view := ansi.Strip(model.themePickerView(80, 10))
	for _, want := range []string{"> zz-custom"} {
		if !strings.Contains(view, want) {
			t.Fatalf("theme picker = %q, missing %q", view, want)
		}
	}
	if strings.Contains(view, "Themes") || strings.Contains(view, "enter apply") || strings.Contains(view, "esc close") {
		t.Fatalf("theme picker retains redundant labels: %q", view)
	}
}

func hasAction(actions []config.Action, name string) bool {
	for _, action := range actions {
		if action.Name == name {
			return true
		}
	}
	return false
}
