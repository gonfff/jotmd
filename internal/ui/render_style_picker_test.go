package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/config"
)

func TestRenderStylePickerPreviewsRestoresAndApplies(t *testing.T) {
	model := newModel(testStore(t), config.Defaults(), jotmdTheme(t))
	model = updateModel(t, model, key("s"))
	if model.mode != RenderStylePicker || model.preview.renderStyle != "quiet" {
		t.Fatalf("opened picker = (mode=%v, style=%q)", model.mode, model.preview.renderStyle)
	}
	model = updateModel(t, model, key("down"))
	if model.preview.renderStyle != "surface" || !strings.Contains(ansi.Strip(model.renderStylePickerView(80, 10)), "> surface") {
		t.Fatalf("preview style = %q, want surface", model.preview.renderStyle)
	}
	model = updateModel(t, model, key("esc"))
	if model.mode != Browse || model.preview.renderStyle != "quiet" {
		t.Fatalf("closed picker = (mode=%v, style=%q), want restored quiet", model.mode, model.preview.renderStyle)
	}

	model = updateModel(t, model, key("s"))
	model = updateModel(t, model, key("down"))
	model = updateModel(t, model, key("enter"))
	if model.mode != Browse || model.preview.renderStyle != "surface" || model.cfg.Preview.RenderStyle != "quiet" {
		t.Fatalf("applied picker = (mode=%v, style=%q, config=%q)", model.mode, model.preview.renderStyle, model.cfg.Preview.RenderStyle)
	}
}

func TestRenderStylePickerUsesConfiguredStyleAndBinding(t *testing.T) {
	cfg := config.Defaults()
	cfg.Preview.RenderStyle = "structural"
	model := NewModelWithOptions(testStore(t), cfg, jotmdTheme(t), config.Keymap{Bindings: []config.Binding{{Action: "preview.style", Keys: []string{"S"}}}}, nil)
	model = updateModel(t, model, key("s"))
	if model.mode != Browse {
		t.Fatal("default style binding remained active after override")
	}
	model = updateModel(t, model, key("S"))
	if model.mode != RenderStylePicker || !strings.Contains(ansi.Strip(model.renderStylePickerView(80, 10)), "> structural") {
		t.Fatalf("configured picker = (mode=%v, style=%q)", model.mode, model.preview.renderStyle)
	}
}

func TestRenderStylePickerReloadRestoresReloadedStyle(t *testing.T) {
	model := newModel(testStore(t), config.Defaults(), jotmdTheme(t))
	model = updateModel(t, model, key("s"))
	model = updateModel(t, model, key("down"))
	cfg := config.Defaults()
	cfg.Preview.RenderStyle = "structural"
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{Config: cfg, theme: jotmdTheme(t)}})
	model = updateModel(t, model, key("esc"))
	if model.mode != Browse || model.preview.renderStyle != "structural" {
		t.Fatalf("picker after reload = (mode=%v, style=%q)", model.mode, model.preview.renderStyle)
	}
}

func TestRenderStylePickerReloadKeepsQuery(t *testing.T) {
	model := newModel(testStore(t), config.Defaults(), jotmdTheme(t))
	model = updateModel(t, model, key("s"))
	model = updateModel(t, model, key("f"))
	model = updateModel(t, model, key("a"))
	cfg := config.Defaults()
	cfg.Preview.RenderStyle = "structural"
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{Config: cfg, theme: jotmdTheme(t)}})
	if !strings.Contains(ansi.Strip(model.statusView(80, ContextTheme)), "Render style: fa") || !strings.Contains(ansi.Strip(model.renderStylePickerView(80, 10)), "surface") {
		t.Fatalf("picker reload lost query: status=%q view=%q", model.statusView(80, ContextTheme), model.renderStylePickerView(80, 10))
	}
	if model.preview.renderStyle != "surface" {
		t.Fatalf("picker reload preview = %q, want filtered surface", model.preview.renderStyle)
	}
}

func TestRenderStylePickerFiltersAndNavigatesWithoutTreatingPrintableKeysAsBindings(t *testing.T) {
	cfg := config.Defaults()
	cfg.StatusBar = false
	model := newModel(testStore(t), cfg, jotmdTheme(t))
	model.width, model.height = 80, 16
	model = updateModel(t, model, key("s"))
	model = updateModel(t, model, key("u"))
	model = updateModel(t, model, key("r"))
	view := ansi.Strip(model.View().Content)
	for _, want := range []string{"Render style: ur", "> surface"} {
		if !strings.Contains(view, want) {
			t.Fatalf("picker view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(model.renderStylePickerView(80, 10), "quiet") {
		t.Fatalf("filtered picker includes non-match: %q", model.renderStylePickerView(80, 10))
	}
	model = updateModel(t, model, key("ctrl+n"))
	if model.preview.renderStyle != "structural" {
		t.Fatalf("ctrl+n preview = %q, want structural", model.preview.renderStyle)
	}
	model = updateModel(t, model, key("ctrl+p"))
	if model.preview.renderStyle != "surface" {
		t.Fatalf("ctrl+p preview = %q, want surface", model.preview.renderStyle)
	}
	model = updateModel(t, model, key("esc"))
	if model.preview.renderStyle != "quiet" || model.mode != Browse {
		t.Fatalf("escape picker = (mode=%v, style=%q), want Browse/quiet", model.mode, model.preview.renderStyle)
	}
	model = updateModel(t, model, key("s"))
	model = updateModel(t, model, key("z"))
	if !strings.Contains(ansi.Strip(model.renderStylePickerView(80, 10)), "No matches.") {
		t.Fatalf("no-match picker = %q", model.renderStylePickerView(80, 10))
	}
	model = updateModel(t, model, key("enter"))
	if model.mode != RenderStylePicker || model.preview.renderStyle != "quiet" {
		t.Fatalf("enter on no matches = (mode=%v, style=%q), want open/quiet", model.mode, model.preview.renderStyle)
	}
}
