package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/ui"
)

var actions = []config.Action{
	{Name: "tree.down", Contexts: []string{"tree"}},
	{Name: "tree.up", Contexts: []string{"tree"}},
	{Name: "help.close", Contexts: []string{"help"}},
}

func TestInitCreatesTemplatesWithoutOverwriting(t *testing.T) {
	home := t.TempDir()
	configPath := filepath.Join(home, ".config", "jotmd", "config.toml")
	keymapPath := filepath.Join(home, ".config", "jotmd", "keybindings.toml")
	if err := config.InitAt(configPath); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{configPath, keymapPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat(%q): %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%q mode = %o, want 600", path, info.Mode().Perm())
		}
	}
	template, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(template), `# render_style = "quiet"`) {
		t.Fatalf("config template is missing render_style: %q", template)
	}
	if err := os.WriteFile(configPath, []byte("notes_dir = \"/edited\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.InitAt(configPath); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second InitAt() error = %v, want already exists", err)
	}
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "notes_dir = \"/edited\"\n" {
		t.Errorf("config template was overwritten: %q", contents)
	}
}

func TestInitKeymapTemplateLoadsEveryWorkflowBinding(t *testing.T) {
	home := t.TempDir()
	if err := config.InitAt(filepath.Join(home, ".config", "jotmd", "config.toml")); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"tree.open":     "enter,space",
		"note.edit":     "e",
		"note.new":      "n",
		"directory.new": "N",
		"note.rename":   "r",
		"note.copy":     "c",
		"note.move":     "m",
		"note.trash":    "d",
		"note.delete":   "D",
		"search.open":   "/",
		"app.palette":   ":",
		"preview.raw":   "v",
		"preview.wrap":  "w",
		"preview.style": "s",
	}
	registry := ui.ActionRegistry()
	for index := range registry {
		if _, required := want[registry[index].Name]; required {
			registry[index].Keys = nil
		}
	}
	keymap, err := config.LoadKeymap(filepath.Join(home, ".config", "jotmd", "keybindings.toml"), registry)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string, len(keymap.Bindings))
	for _, binding := range keymap.Bindings {
		got[binding.Action] = strings.Join(binding.Keys, ",")
	}
	for action, keys := range want {
		if got[action] != keys {
			t.Errorf("template binding %q = %q, want %q", action, got[action], keys)
		}
	}
}

func TestLoadKeymapOverridesAction(t *testing.T) {
	path := writeFile(t, filepath.Join(t.TempDir(), "keybindings.toml"), "[bindings]\n\"tree.down\" = [\"n\"]\n")
	keymap, err := config.LoadKeymap(path, actions)
	if err != nil {
		t.Fatal(err)
	}
	if got := keymap.Bindings[0]; got.Action != "tree.down" || strings.Join(got.Keys, ",") != "n" {
		t.Errorf("binding = %#v, want tree.down = n", got)
	}
}

func TestLoadKeymapRejectsDuplicateContextualKeys(t *testing.T) {
	path := writeFile(t, filepath.Join(t.TempDir(), "keybindings.toml"), "[bindings]\n\"tree.down\" = [\"j\"]\n\"tree.up\" = [\"j\"]\n")
	_, err := config.LoadKeymap(path, actions)
	if err == nil || !strings.Contains(err.Error(), "tree") || !strings.Contains(err.Error(), "j") {
		t.Fatalf("LoadKeymap() error = %v, want duplicate tree key", err)
	}
}

func TestLoadKeymapRejectsEmptyKey(t *testing.T) {
	path := writeFile(t, filepath.Join(t.TempDir(), "keybindings.toml"), "[bindings]\n\"tree.down\" = [\"\"]\n")
	_, err := config.LoadKeymap(path, actions)
	if err == nil || !strings.Contains(err.Error(), "empty key") {
		t.Fatalf("LoadKeymap() error = %v, want empty key", err)
	}
}

func TestLoadKeymapEmptyArrayDisablesActionAndKeepsDefaults(t *testing.T) {
	configured := []config.Action{
		{Name: "tree.down", Keys: []string{"j", "down"}, Contexts: []string{"tree"}},
		{Name: "tree.up", Keys: []string{"k", "up"}, Contexts: []string{"tree"}},
	}
	path := writeFile(t, filepath.Join(t.TempDir(), "keybindings.toml"), "[bindings]\n\"tree.down\" = []\n")
	keymap, err := config.LoadKeymap(path, configured)
	if err != nil {
		t.Fatal(err)
	}
	if len(keymap.Bindings[0].Keys) != 0 {
		t.Errorf("tree.down keys = %#v, want disabled", keymap.Bindings[0].Keys)
	}
	if strings.Join(keymap.Bindings[1].Keys, ",") != "k,up" {
		t.Errorf("tree.up keys = %#v, want defaults", keymap.Bindings[1].Keys)
	}
}

func TestLoadKeymapRejectsInvalidBubbleTeaNotation(t *testing.T) {
	for _, key := range []string{"ctrl++a", "ctrl+a+b", "not-a-key", "escape", "kpplus", "ctrl+leftctrl"} {
		t.Run(key, func(t *testing.T) {
			path := writeFile(t, filepath.Join(t.TempDir(), "keybindings.toml"), "[bindings]\n\"tree.down\" = [\""+key+"\"]\n")
			if _, err := config.LoadKeymap(path, actions); err == nil || !strings.Contains(err.Error(), "invalid key") {
				t.Fatalf("LoadKeymap() error = %v, want invalid key notation", err)
			}
		})
	}
}

func TestLoadKeymapAcceptsUsefulBubbleTeaNotation(t *testing.T) {
	configured := []config.Action{{Name: "tree.down", Keys: []string{"j"}, Contexts: []string{"tree"}}}
	path := writeFile(t, filepath.Join(t.TempDir(), "keybindings.toml"), "[bindings]\n\"tree.down\" = [\"ctrl+j\", \"alt+shift+f2\", \"Ж\", \"+\"]\n")
	if _, err := config.LoadKeymap(path, configured); err != nil {
		t.Fatal(err)
	}
}

func TestLoadKeymapRejectsUnknownActionAndOverlappingContextDuplicate(t *testing.T) {
	unknown := writeFile(t, filepath.Join(t.TempDir(), "keybindings.toml"), "[bindings]\n\"unknown\" = [\"x\"]\n")
	if _, err := config.LoadKeymap(unknown, actions); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("LoadKeymap() unknown action error = %v", err)
	}

	overlapping := []config.Action{
		{Name: "note.edit", Keys: []string{"e"}, Contexts: []string{"tree", "preview"}},
		{Name: "note.rename", Keys: []string{"r"}, Contexts: []string{"preview"}},
	}
	duplicate := writeFile(t, filepath.Join(t.TempDir(), "overlap.toml"), "[bindings]\n\"note.rename\" = [\"e\"]\n")
	if _, err := config.LoadKeymap(duplicate, overlapping); err == nil || !strings.Contains(err.Error(), "preview") {
		t.Fatalf("LoadKeymap() overlapping duplicate error = %v", err)
	}
}

func TestLoadAndDumpKeymapPreservesExistingModalBindings(t *testing.T) {
	configured := []config.Action{
		{Name: "tree.down", Keys: []string{"j"}, Contexts: []string{"tree"}},
		{Name: "help.close", Keys: []string{"esc"}, Contexts: []string{"help"}},
		{Name: "theme.apply", Keys: []string{"enter"}, Contexts: []string{"theme"}},
		{Name: "theme.close", Keys: []string{"esc"}, Contexts: []string{"theme"}},
	}
	oldTemplate := "[bindings]\n\"help.close\" = [\"esc\"]\n\"theme.apply\" = [\"enter\"]\n\"theme.close\" = [\"esc\"]\n"
	keymap, err := config.LoadKeymap(writeFile(t, filepath.Join(t.TempDir(), "keybindings.toml"), oldTemplate), configured)
	if err != nil {
		t.Fatal(err)
	}
	var dumped bytes.Buffer
	if err := config.DumpKeymap(&dumped, keymap); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"help.close", "theme.apply", "theme.close"} {
		if !strings.Contains(dumped.String(), `"`+action+`"`) {
			t.Errorf("DumpKeymap() = %q, want %q preserved", dumped.String(), action)
		}
	}
	path := writeFile(t, filepath.Join(t.TempDir(), "keybindings.toml"), dumped.String())
	if _, err := config.LoadKeymap(path, configured); err != nil {
		t.Fatalf("LoadKeymap(DumpKeymap()) error = %v", err)
	}
}
