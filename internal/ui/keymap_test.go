package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gonfff/jotmd/internal/config"
)

func TestBindingsForKeymapDisablesDefaultAction(t *testing.T) {
	bindings := BindingsForKeymap(config.Keymap{Bindings: []config.Binding{{Action: "app.quit", Keys: []string{}}}})
	if action, ok := actionForKey(bindings, "q", ContextTree); ok || action != "" {
		t.Fatalf("actionForKey(q) = (%q, %t), want disabled", action, ok)
	}
}

func TestLoadedBubbleTeaKeysResolveRuntimeMessages(t *testing.T) {
	tests := []struct {
		name       string
		actionName string
		context    Context
		want       Action
		message    tea.KeyPressMsg
	}{
		{name: "ctrl plus", actionName: "tree.down", context: ContextTree, want: ActionDown, message: tea.KeyPressMsg(tea.Key{Code: '+', Mod: tea.ModCtrl})},
		{name: "keypad plus", actionName: "tree.down", context: ContextTree, want: ActionDown, message: tea.KeyPressMsg(tea.Key{Code: tea.KeyKpPlus})},
		{name: "media play", actionName: "tree.down", context: ContextTree, want: ActionDown, message: tea.KeyPressMsg(tea.Key{Code: tea.KeyMediaPlay})},
		{name: "space", actionName: "tree.open", context: ContextTree, want: ActionOpen, message: tea.KeyPressMsg(tea.Key{Code: tea.KeySpace})},
		{name: "raw", actionName: "preview.raw", context: ContextPreview, want: ActionPreviewRaw, message: tea.KeyPressMsg(tea.Key{Text: "v"})},
		{name: "wrap", actionName: "preview.wrap", context: ContextPreview, want: ActionPreviewWrap, message: tea.KeyPressMsg(tea.Key{Text: "w"})},
		{name: "style", actionName: "preview.style", context: ContextPreview, want: ActionPreviewStyle, message: tea.KeyPressMsg(tea.Key{Text: "s"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "keybindings.toml")
			contents := fmt.Sprintf("[bindings]\n%q = [%q]\n", tt.actionName, tt.message.String())
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			keymap, err := config.LoadKeymap(path, ActionRegistry())
			if err != nil {
				t.Fatal(err)
			}
			if action, ok := actionForKey(BindingsForKeymap(keymap), tt.message.String(), tt.context); !ok || action != tt.want {
				t.Fatalf("actionForKey(%q) = (%q, %t), want %q", tt.message.String(), action, ok, tt.want)
			}
		})
	}
}

func TestThemePickerDefaultBindingsUseNonPrintableNavigation(t *testing.T) {
	for _, test := range []struct {
		key  string
		want Action
	}{
		{key: "up", want: ActionThemeUp},
		{key: "ctrl+p", want: ActionThemeUp},
		{key: "down", want: ActionThemeDown},
		{key: "ctrl+n", want: ActionThemeDown},
		{key: "enter", want: ActionThemeApply},
		{key: "esc", want: ActionThemeClose},
	} {
		if action, ok := actionForKey(DefaultBindings(), test.key, ContextTheme); !ok || action != test.want {
			t.Fatalf("actionForKey(%q) = (%q, %t), want %q", test.key, action, ok, test.want)
		}
	}
	for _, key := range []string{"j", "k"} {
		if _, ok := actionForKey(DefaultBindings(), key, ContextTheme); ok {
			t.Fatalf("actionForKey(%q) unexpectedly binds picker navigation", key)
		}
	}
}

func TestTOCUsesTreeNavigationBindings(t *testing.T) {
	for _, test := range []struct {
		key  string
		want Action
	}{
		{key: "k", want: ActionUp},
		{key: "up", want: ActionUp},
		{key: "j", want: ActionDown},
		{key: "down", want: ActionDown},
		{key: "h", want: ActionCollapse},
		{key: "left", want: ActionCollapse},
		{key: "l", want: ActionExpand},
		{key: "right", want: ActionExpand},
	} {
		if action, ok := actionForKey(DefaultBindings(), test.key, ContextTOC); !ok || action != test.want {
			t.Fatalf("actionForKey(%q, toc) = (%q, %t), want %q", test.key, action, ok, test.want)
		}
	}
}
