package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/gonfff/jotmd/internal/config"
)

const (
	usUnshifted = "`1234567890-=[]\\;',./"
	usShifted   = "~!@#$%^&*()_+{}|:\"<>?"
)

func DefaultBindings() []Binding {
	return []Binding{
		{Name: "tree.up", Action: ActionUp, Keys: []string{"k", "up"}, Label: "up", Contexts: []Context{ContextTree, ContextTOC}},
		{Name: "tree.down", Action: ActionDown, Keys: []string{"j", "down"}, Label: "down", Contexts: []Context{ContextTree, ContextTOC}},
		{Name: "tree.first", Action: ActionFirst, Keys: []string{"home"}, Label: "first", Contexts: []Context{ContextTree}},
		{Name: "tree.last", Action: ActionLast, Keys: []string{"end"}, Label: "last", Contexts: []Context{ContextTree}},
		{Name: "tree.collapse", Action: ActionCollapse, Keys: []string{"h", "left"}, Label: "collapse", Contexts: []Context{ContextTree, ContextTOC}},
		{Name: "tree.expand", Action: ActionExpand, Keys: []string{"l", "right"}, Label: "expand", Contexts: []Context{ContextTree, ContextTOC}},
		{Name: "tree.parent", Action: ActionParent, Keys: []string{"backspace"}, Label: "parent", Contexts: []Context{ContextTree}},
		{Name: "tree.open", Action: ActionOpen, Keys: []string{"enter", "space"}, Label: "open", Contexts: []Context{ContextTree, ContextTOC}},
		{Name: "note.edit", Action: ActionEdit, Keys: []string{"e"}, Label: "edit", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "note.new", Action: ActionNewNote, Keys: []string{"n"}, Label: "new note", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "directory.new", Action: ActionNewDirectory, Keys: []string{"N"}, Label: "new directory", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "note.rename", Action: ActionRename, Keys: []string{"r"}, Label: "rename", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "note.copy", Action: ActionCopy, Keys: []string{"c"}, Label: "copy", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "note.move", Action: ActionMove, Keys: []string{"m"}, Label: "move", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "note.trash", Action: ActionTrash, Keys: []string{"d"}, Label: "trash", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "note.delete", Action: ActionDelete, Keys: []string{"D"}, Label: "delete", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "app.focus", Action: ActionFocus, Keys: []string{"tab"}, Label: "focus", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "app.reload", Action: ActionReload, Keys: []string{"R"}, Label: "reload", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "app.help", Action: ActionHelp, Keys: []string{"?"}, Label: "help", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "theme.select", Action: ActionThemeSelect, Keys: []string{"t"}, Label: "theme", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "search.open", Action: ActionSearch, Keys: []string{"/"}, Label: "search", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "app.palette", Action: ActionPalette, Keys: []string{"P"}, Label: "commands", Contexts: []Context{ContextTree, ContextTOC, ContextPreview, ContextSearch}},
		{Name: "preview.raw", Action: ActionPreviewRaw, Keys: []string{"v"}, Label: "raw", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "preview.wrap", Action: ActionPreviewWrap, Keys: []string{"w"}, Label: "wrap", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "preview.style", Action: ActionPreviewStyle, Keys: []string{"s"}, Label: "render style", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "view.agent_memory", Action: ActionAgentMemory, Keys: []string{"a"}, Label: "agent memory", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "app.quit", Action: ActionQuit, Keys: []string{"q"}, Label: "quit", Contexts: []Context{ContextTree, ContextTOC, ContextPreview}},
		{Name: "help.close", Action: ActionClose, Keys: []string{"esc"}, Label: "close", Contexts: []Context{ContextHelp}},
		{Name: "theme.up", Action: ActionThemeUp, Keys: []string{"up", "ctrl+p"}, Label: "up", Contexts: []Context{ContextTheme}},
		{Name: "theme.down", Action: ActionThemeDown, Keys: []string{"down", "ctrl+n"}, Label: "down", Contexts: []Context{ContextTheme}},
		{Name: "theme.apply", Action: ActionThemeApply, Keys: []string{"enter"}, Label: "apply", Contexts: []Context{ContextTheme}},
		{Name: "theme.close", Action: ActionThemeClose, Keys: []string{"esc"}, Label: "close", Contexts: []Context{ContextTheme}},
	}
}

func BindingsForKeymap(keymap config.Keymap) []Binding {
	overrides := make(map[string][]string, len(keymap.Bindings))
	for _, binding := range keymap.Bindings {
		overrides[binding.Action] = binding.Keys
	}
	bindings := DefaultBindings()
	for index := range bindings {
		if keys, ok := overrides[bindings[index].Name]; ok {
			bindings[index].Keys = append([]string(nil), keys...)
		}
	}
	return bindings
}

func actionForKeyPress(bindings []Binding, message tea.KeyPressMsg, context Context) (Action, bool) {
	if action, ok := actionForKey(bindings, message.String(), context); ok {
		return action, true
	}
	return actionForKey(bindings, physicalKey(message).String(), context)
}

func physicalKey(message tea.KeyPressMsg) tea.KeyPressMsg {
	key := message.Key()
	if key.BaseCode == 0 {
		return message
	}
	key.Code = key.BaseCode
	key.BaseCode = 0
	key.ShiftedCode = 0
	key.Text = ""
	key.Mod &^= tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
	if key.Mod == tea.ModShift {
		shifted := rune(0)
		if key.Code >= 'a' && key.Code <= 'z' {
			shifted = key.Code - 'a' + 'A'
		} else if index := strings.IndexRune(usUnshifted, key.Code); index >= 0 {
			shifted = rune(usShifted[index])
		}
		if shifted != 0 {
			key.Code = shifted
			key.Mod = 0
			key.Text = string(shifted)
		}
	}
	return tea.KeyPressMsg(key)
}

func actionForKey(bindings []Binding, key string, context Context) (Action, bool) {
	for _, binding := range bindings {
		if !hasContext(binding.Contexts, context) {
			continue
		}
		for _, candidate := range binding.Keys {
			if key == candidate {
				return binding.Action, true
			}
		}
	}
	return "", false
}

func hasContext(contexts []Context, want Context) bool {
	for _, context := range contexts {
		if context == want {
			return true
		}
	}
	return false
}
