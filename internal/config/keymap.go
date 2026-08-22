package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/BurntSushi/toml"
)

type Action struct {
	Name     string
	Keys     []string
	Contexts []string
}

type Binding struct {
	Action string
	Keys   []string
}

type Keymap struct {
	Bindings []Binding
}

type keymapFile struct {
	Bindings map[string][]string `toml:"bindings"`
}

var bubbleTeaSpecialKeys = func() map[string]rune {
	keys := make(map[string]rune)
	for code := rune(tea.KeyUp); code <= rune(tea.KeyIsoLevel5Shift); code++ {
		keys[tea.KeyPressMsg(tea.Key{Code: code}).String()] = code
	}
	for _, code := range []rune{tea.KeyBackspace, tea.KeyTab, tea.KeyEnter, tea.KeyEscape, tea.KeySpace} {
		keys[tea.KeyPressMsg(tea.Key{Code: code}).String()] = code
	}
	return keys
}()

func LoadKeymap(path string, actions []Action) (Keymap, error) {
	defaults := make(map[string]Action, len(actions))
	for _, action := range actions {
		defaults[action.Name] = Action{
			Name:     action.Name,
			Keys:     append([]string(nil), action.Keys...),
			Contexts: append([]string(nil), action.Contexts...),
		}
	}

	contents, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Keymap{}, fmt.Errorf("read keybindings %q: %w", path, err)
	}
	var overrides keymapFile
	if err == nil {
		metadata, decodeErr := toml.Decode(string(contents), &overrides)
		if decodeErr != nil {
			return Keymap{}, fmt.Errorf("decode keybindings %q: %w", path, decodeErr)
		}
		if unknown := metadata.Undecoded(); len(unknown) != 0 {
			return Keymap{}, fmt.Errorf("unknown keybindings field: %s", unknown[0])
		}
		for name, keys := range overrides.Bindings {
			action, ok := defaults[name]
			if !ok {
				return Keymap{}, fmt.Errorf("unknown keybinding action %q", name)
			}
			for _, key := range keys {
				if key == "" {
					return Keymap{}, fmt.Errorf("keybinding action %q has an empty key", name)
				}
				if !validKey(key) {
					return Keymap{}, fmt.Errorf("keybinding action %q has invalid key %q", name, key)
				}
			}
			action.Keys = append([]string(nil), keys...)
			defaults[name] = action
		}
	}

	keymap := Keymap{Bindings: make([]Binding, 0, len(actions))}
	seen := make(map[string]string)
	for _, original := range actions {
		action := defaults[original.Name]
		for _, context := range action.Contexts {
			for _, key := range action.Keys {
				if prior, ok := seen[context+"\x00"+key]; ok {
					return Keymap{}, fmt.Errorf("duplicate key %q in %s context for %q and %q", key, context, prior, action.Name)
				}
				seen[context+"\x00"+key] = action.Name
			}
		}
		keymap.Bindings = append(keymap.Bindings, Binding{Action: action.Name, Keys: action.Keys})
	}
	return keymap, nil
}

func DumpKeymap(w io.Writer, keymap Keymap) error {
	bindings := make(map[string][]string, len(keymap.Bindings))
	for _, binding := range keymap.Bindings {
		bindings[binding.Action] = binding.Keys
	}
	if err := toml.NewEncoder(w).Encode(keymapFile{Bindings: bindings}); err != nil {
		return fmt.Errorf("dump keybindings: %w", err)
	}
	return nil
}

func validKey(key string) bool {
	original := key
	var modifiers tea.KeyMod
	for _, modifier := range []struct {
		prefix string
		value  tea.KeyMod
	}{
		{prefix: "ctrl+", value: tea.ModCtrl},
		{prefix: "alt+", value: tea.ModAlt},
		{prefix: "shift+", value: tea.ModShift},
		{prefix: "meta+", value: tea.ModMeta},
		{prefix: "hyper+", value: tea.ModHyper},
		{prefix: "super+", value: tea.ModSuper},
	} {
		if strings.HasPrefix(key, modifier.prefix) {
			key = strings.TrimPrefix(key, modifier.prefix)
			modifiers |= modifier.value
		}
	}
	code, special := bubbleTeaSpecialKeys[key]
	if !special {
		if utf8.RuneCountInString(key) != 1 || !unicode.IsPrint([]rune(key)[0]) || unicode.IsSpace([]rune(key)[0]) {
			return false
		}
		code = []rune(key)[0]
	}
	return tea.KeyPressMsg(tea.Key{Code: code, Mod: modifiers}).String() == original
}
