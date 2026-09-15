package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/config"
)

func TestActionPaletteUsesTitledWindowedLayout(t *testing.T) {
	model := sizedLoadedModel(t)
	model.width, model.height = 100, 16
	model = updateModel(t, model, key("P"))

	popup := model.popupContent(model.popupSize())
	stripped := ansi.Strip(popup)
	if lipgloss.Width(popup) != 80 || lipgloss.Height(popup) != 12 {
		t.Fatalf("palette popup = %dx%d, want 80x12", lipgloss.Width(popup), lipgloss.Height(popup))
	}
	if !strings.Contains(strings.SplitN(stripped, "\n", 2)[0], "commands (") || !strings.Contains(stripped, "type to filter...") {
		t.Fatalf("palette lacks title or filter hint:\n%s", stripped)
	}

	model.theme.Palette.Heading = "#A1B2C3"
	found := false
	for _, line := range strings.Split(model.paletteView(40, 8), "\n") {
		plain := ansi.Strip(line)
		if strings.HasPrefix(plain, "> ") {
			found = true
			keys := strings.Join(model.palette.items[0].binding.Keys, "/")
			if ansi.StringWidth(line) != 40 || strings.LastIndex(plain, keys) != 26 {
				t.Fatalf("selected palette row width/key column = %d/%d, want 40/26: %q", ansi.StringWidth(line), strings.LastIndex(plain, keys), plain)
			}
			break
		}
	}
	if !found {
		t.Fatal("palette has no selected row")
	}
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(model.theme.Palette.Heading))
	if view := model.paletteView(78, 10); !strings.Contains(view, keyStyle.Render("c")) {
		t.Fatalf("palette shortcut does not use heading color: %q", view)
	}
}

func TestActionPaletteUsesEffectiveRegistryMetadata(t *testing.T) {
	model := NewModelWithOptions(testStore(t), config.Defaults(), jotmdTheme(t), config.Keymap{Bindings: []config.Binding{
		{Action: "app.palette", Keys: []string{"ctrl+p"}},
		{Action: "note.edit", Keys: []string{"ctrl+e"}},
	}}, nil)
	model = updateModel(t, model, run(model.Init()))
	model = updateModel(t, model, tea.WindowSizeMsg{Width: 100, Height: 16})

	model = updateModel(t, model, key("ctrl+p"))
	if model.mode == Browse {
		t.Fatal("palette did not enter an overlay mode")
	}
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "edit") || !strings.Contains(view, "ctrl+e") {
		t.Fatalf("palette view = %q, want effective edit metadata", view)
	}
	for _, item := range model.palette.items {
		if !hasBinding(model.bindings, item.binding) || !hasAnyContext(item.binding.Contexts, ContextTree, ContextPreview, ContextSearch) {
			t.Fatalf("palette item %#v is not a displayable registered binding", item.binding)
		}
	}
	if !hasAction(ActionRegistry(), "app.palette") {
		t.Fatal("app.palette is not registered")
	}
	model = updateModel(t, model, key("apply"))
	if view := ansi.Strip(model.View().Content); !strings.Contains(view, "No actions.") {
		t.Fatalf("palette exposed modal apply control: %q", view)
	}
	model = updateModel(t, model, key("esc"))
	model = updateModel(t, model, key("?"))
	if help := ansi.Strip(model.View().Content); !strings.Contains(help, "ctrl+e  edit") {
		t.Fatalf("help did not use the effective action metadata: %q", help)
	}
}

func hasBinding(bindings []Binding, want Binding) bool {
	for _, binding := range bindings {
		if binding.Name == want.Name && binding.Action == want.Action && binding.Label == want.Label &&
			strings.Join(binding.Keys, "\x00") == strings.Join(want.Keys, "\x00") {
			return true
		}
	}
	return false
}

func TestActionPaletteFiltersDisablesDispatchesAndCancels(t *testing.T) {
	model := sizedLoadedModel(t)
	model.editorCommand.Executable = ""
	model = updateModel(t, model, key("P"))
	model = updateModel(t, model, key("edit"))
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "edit") || !strings.Contains(view, "editor not configured") {
		t.Fatalf("filtered palette = %q, want disabled edit", view)
	}
	model = updateModel(t, model, key("enter"))
	if model.mode == Browse {
		t.Fatal("disabled action closed the palette")
	}
	model = updateModel(t, model, key("esc"))
	if model.mode != Browse {
		t.Fatalf("escape mode = %v, want Browse", model.mode)
	}

	model = sizedLoadedModel(t)
	model = updateModel(t, model, key("P"))
	model = updateModel(t, model, key("new note"))
	model = updateModel(t, model, key("enter"))
	if model.mode != NewNotePrompt {
		t.Fatalf("enter mode = %v, want NewNotePrompt", model.mode)
	}
}

func TestActionPaletteEnablesCopyAndMoveForDirectory(t *testing.T) {
	model := sizedLoadedModel(t)
	model.tree, _ = model.tree.Select("docs")
	for _, action := range []Action{ActionCopy, ActionMove} {
		if enabled, reason := model.paletteActionState(action); !enabled {
			t.Errorf("%s enabled = false (%q), want true", action, reason)
		}
	}
}

func TestActionPaletteOpensFromSearchAndOmitsModalControls(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("tab"))
	model = updateModel(t, model, key("P"))
	if model.mode != CommandPalette || model.focus != ContextPreview {
		t.Fatalf("preview palette state = (%v, %v), want CommandPalette from preview", model.mode, model.focus)
	}
	model = updateModel(t, model, key("esc"))

	model = updateModel(t, model, key("/"))
	model = updateModel(t, model, key("P"))
	if model.mode == SearchPrompt {
		t.Fatal("search Shift+P did not enter the palette")
	}
	if view := ansi.Strip(model.View().Content); strings.Contains(view, "theme.apply") || strings.Contains(view, "help.close") {
		t.Fatalf("palette view exposed modal controls: %q", view)
	}
	model = updateModel(t, model, key("esc"))
	if model.mode != SearchPrompt {
		t.Fatalf("palette escape mode = %v, want SearchPrompt", model.mode)
	}
}

func TestActionPaletteShowsReloadedEffectiveKeys(t *testing.T) {
	model := sizedLoadedModel(t)
	model = updateModel(t, model, key("P"))
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{
		Config: config.Defaults(),
		keymap: config.Keymap{Bindings: []config.Binding{
			{Action: "note.edit", Keys: []string{"ctrl+e"}},
		}},
		theme: jotmdTheme(t),
	}})
	view := ansi.Strip(model.View().Content)
	if !strings.Contains(view, "ctrl+e") || strings.Contains(view, "edit  e") {
		t.Fatalf("reloaded palette keys = %q, want ctrl+e only", view)
	}
}
