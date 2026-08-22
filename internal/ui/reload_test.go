package ui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/theme"
)

func TestModelAppliesResolvedConfigurationAsOneValue(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{})
	accepted := jotmdTheme(t)
	accepted.Name = "accepted-in-memory"
	model.theme = accepted
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{
		Config: config.Config{
			NotesDir: "/notes", Theme: "nord", TreeWidth: 40, Ignore: []string{".git"}, Sort: "name", DirectoriesFirst: true,
			StatusBar: false, Watch: true, Preview: config.Preview{Wrap: false, MaxBytes: 2 * 1024 * 1024, RenderStyle: "structural"},
		},
		keymap: config.Keymap{Bindings: []config.Binding{{Action: "tree.down", Keys: []string{"n"}}}},
		theme:  builtinTheme(t, "nord"),
	}})

	if model.cfg.Theme != "nord" || model.cfg.TreeWidth != 40 || model.cfg.StatusBar || model.theme.Name != "nord" || model.preview.wrap || model.preview.renderStyle != "structural" {
		t.Fatalf("resolved UI config = (%#v, %q, wrap=%t)", model.cfg, model.theme.Name, model.preview.wrap)
	}
	if action, ok := actionForKey(model.bindings, "n", ContextTree); !ok || action != ActionDown {
		t.Fatalf("reloaded keymap action = (%q, %t), want down", action, ok)
	}
	if strings.Contains(ansi.Strip(model.View().Content), "n down") {
		t.Fatal("status bar remained visible after atomic reload")
	}
}

func TestModelReloadAppliesSupportedFieldsAndKeepsInfrastructureOwnedValues(t *testing.T) {
	startup := config.Defaults()
	startup.NotesDir = "/startup-notes"
	startup.Editor = []string{"startup-editor", "--wait"}
	startup.Ignore = []string{"startup-ignore"}
	startup.ShowHidden = true
	startup.Watch = false
	startup.Preview.MaxBytes = 4096
	model := NewModelWithOptions(testStore(t), startup, jotmdTheme(t), config.Keymap{}, nil)
	reloadedTheme := builtinTheme(t, "nord")
	reloadedTheme.Palette.Accent = "#FFFFFF"
	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{
		Config: config.Config{
			NotesDir: "/reloaded-notes", Editor: []string{"reloaded-editor"}, Theme: "nord", ThemeColors: map[string]string{"accent": "#FFFFFF"},
			TreeWidth: 45, NoColor: false, ShowHidden: false, Ignore: []string{"reloaded-ignore"}, Sort: "name", DirectoriesFirst: true,
			StatusBar: false, Watch: true, Preview: config.Preview{Wrap: false, MaxBytes: 8192},
		},
		keymap: config.Keymap{Bindings: []config.Binding{{Action: "tree.down", Keys: []string{"ctrl+n"}}}},
		theme:  reloadedTheme,
	}})

	if model.cfg.Theme != "nord" || model.cfg.ThemeColors["accent"] != "#FFFFFF" || model.cfg.TreeWidth != 45 || model.cfg.StatusBar || !model.cfg.Watch || model.cfg.Preview.Wrap {
		t.Fatalf("supported config fields were not applied: %#v", model.cfg)
	}
	if model.theme != reloadedTheme || model.preview.wrap {
		t.Fatalf("supported runtime values = (%#v, wrap=%t), want reloaded", model.theme, model.preview.wrap)
	}
	if action, ok := actionForKey(model.bindings, "ctrl+n", ContextTree); !ok || action != ActionDown {
		t.Fatalf("reloaded keymap action = (%q, %t), want down", action, ok)
	}
	if model.cfg.NotesDir != "/startup-notes" || !reflect.DeepEqual(model.cfg.Editor, []string{"startup-editor", "--wait"}) ||
		!reflect.DeepEqual(model.cfg.Ignore, []string{"startup-ignore"}) || !model.cfg.ShowHidden || model.cfg.Preview.MaxBytes != 4096 || model.preview.maxBytes != 4096 {
		t.Fatalf("reload replaced infrastructure-owned values: config=%#v preview.maxBytes=%d", model.cfg, model.preview.maxBytes)
	}
}

func TestModelInvalidConfigurationPreservesEveryEffectiveValueAndDiagnostic(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{Bindings: []config.Binding{{Action: "tree.down", Keys: []string{"n"}}}})
	wantConfig, wantTheme, wantBindings := model.cfg, model.theme, append([]Binding(nil), model.bindings...)
	model = updateModel(t, model, configReloadResult{err: errors.New("decode config: broken TOML")})

	if !reflect.DeepEqual(model.cfg, wantConfig) || model.theme != wantTheme || !equalBindings(model.bindings, wantBindings) {
		t.Fatalf("invalid reload changed effective state: (%#v, %#v, %#v)", model.cfg, model.theme, model.bindings)
	}
	model.status = "scan complete"
	if status := ansi.Strip(model.statusView(120, ContextTree)); !strings.Contains(status, "config reload: decode config: broken TOML") {
		t.Fatalf("persistent status = %q, want reload diagnostic", status)
	}

	model = updateModel(t, model, configReloadResult{resolved: resolvedConfig{
		Config: config.Defaults(), keymap: config.Keymap{}, theme: jotmdTheme(t),
	}})
	if model.configWarning != "" {
		t.Fatalf("valid reload kept warning %q", model.configWarning)
	}
}

func TestModelDoesNotApplyResolvedConfigurationOlderThanInvalidEvent(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{})
	wantConfig, wantTheme := model.cfg, model.theme
	model.configReloadGeneration = 1
	model = updateModel(t, model, configWatchResult{
		results: make(chan config.ReloadResult),
		result:  config.ReloadResult{Err: errors.New("newer config is invalid")},
		open:    true,
	})
	model = updateModel(t, model, configReloadResult{
		generation: 1,
		resolved: resolvedConfig{
			Config: config.Config{NotesDir: "/notes", Theme: "nord", TreeWidth: 32, Sort: "name", DirectoriesFirst: true, StatusBar: false, Preview: config.Preview{Wrap: true, MaxBytes: 2 * 1024 * 1024}},
			theme:  builtinTheme(t, "nord"),
		},
	})

	if !reflect.DeepEqual(model.cfg, wantConfig) || model.theme != wantTheme || !strings.Contains(model.configWarning, "newer config is invalid") {
		t.Fatalf("late valid reload replaced newer invalid state: (%#v, %q, %q)", model.cfg, model.theme.Name, model.configWarning)
	}
}

func TestModelDoesNotApplyResolvedConfigurationAfterWatcherStops(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{})
	wantConfig, wantTheme := model.cfg, model.theme
	model.configReloadGeneration = 1
	model = updateModel(t, model, configWatchResult{open: false})
	model = updateModel(t, model, configReloadResult{
		generation: 1,
		resolved: resolvedConfig{
			Config: config.Config{NotesDir: "/notes", Theme: "nord", TreeWidth: 32, Sort: "name", DirectoriesFirst: true, StatusBar: false, Preview: config.Preview{Wrap: true, MaxBytes: 2 * 1024 * 1024}},
			theme:  builtinTheme(t, "nord"),
		},
	})

	if !reflect.DeepEqual(model.cfg, wantConfig) || model.theme != wantTheme || model.configWarning != "config reload: watch stopped" {
		t.Fatalf("late reload replaced watcher-stop state: (%#v, %q, %q)", model.cfg, model.theme.Name, model.configWarning)
	}
}

func TestModelInvalidReloadedThemePreservesResolvedConfiguration(t *testing.T) {
	model := newConfiguredModel(t, config.Keymap{})
	wantConfig, wantTheme := model.cfg, model.theme
	model.resolveTheme = func(config.Config) (theme.Theme, error) {
		return theme.Theme{}, errors.New("unknown theme \"missing\"")
	}
	message := run(model.resolveConfig(1, config.Resolved{
		Config: config.Config{NotesDir: "/notes", Theme: "missing", TreeWidth: 32, Sort: "name", DirectoriesFirst: true, StatusBar: false, Preview: config.Preview{Wrap: true, MaxBytes: 2 * 1024 * 1024}},
		Keymap: config.Keymap{Bindings: []config.Binding{{Action: "tree.down", Keys: []string{"n"}}}},
	}))
	model.configReloadGeneration = 1
	model = updateModel(t, model, message)

	if !reflect.DeepEqual(model.cfg, wantConfig) || model.theme != wantTheme || !strings.Contains(model.configWarning, "unknown theme") {
		t.Fatalf("invalid theme reload changed effective state: (%#v, %q, %q)", model.cfg, model.theme.Name, model.configWarning)
	}
}

func TestModelConfigWarningsResizeHiddenStatusLayout(t *testing.T) {
	tests := []struct {
		name    string
		message tea.Msg
	}{
		{name: "watch start", message: configWatchStarted{err: errors.New("start failed")}},
		{name: "watch stop", message: configWatchResult{open: false}},
		{name: "invalid file", message: configWatchResult{results: make(chan config.ReloadResult), result: config.ReloadResult{Err: errors.New("invalid file")}, open: true}},
		{name: "invalid theme", message: configReloadResult{err: errors.New("invalid theme")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := loadedModel(t)
			model.cfg.StatusBar = false
			model, command := updateModelCommand(t, model, tea.WindowSizeMsg{Width: 120, Height: 20})
			model = updateModel(t, model, run(command))
			withoutWarning := paneHeight(20, false)
			if model.tree.height != withoutWarning || model.preview.viewport.Height() != withoutWarning-2 {
				t.Fatalf("initial geometry = (%d, %d), want (%d, %d)", model.tree.height, model.preview.viewport.Height(), withoutWarning, withoutWarning-2)
			}

			model, _ = updateModelCommand(t, model, test.message)
			withWarning := paneHeight(20, true)
			if model.tree.height != withWarning || model.preview.viewport.Height() != withWarning-2 {
				t.Fatalf("warning geometry = (%d, %d), want (%d, %d)", model.tree.height, model.preview.viewport.Height(), withWarning, withWarning-2)
			}
		})
	}
}

func TestModelStartsConfigWatchFromMissingParent(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing")
	reloader := config.NewReloader(filepath.Join(parent, "config.toml"), func(string) (string, bool) { return "", false }, config.Partial{}, ActionRegistry())
	model := newConfiguredModel(t, config.Keymap{})
	model.SetConfigReloader(&reloader, func(config.Config) (theme.Theme, error) { return jotmdTheme(t), nil })
	message := run(model.startConfigWatch()).(configWatchStarted)
	if message.err != nil {
		t.Fatal(message.err)
	}
	model = updateModel(t, model, message)
	if model.configWarning != "" {
		t.Fatalf("missing-parent warning = %q", model.configWarning)
	}
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		t.Fatalf("config watcher parent = (%v, %v), want directory", info, err)
	}
}

func builtinTheme(t *testing.T, name string) theme.Theme {
	t.Helper()
	th, err := theme.Builtin(name)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

func equalBindings(left, right []Binding) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Name != right[index].Name || strings.Join(left[index].Keys, "\x00") != strings.Join(right[index].Keys, "\x00") {
			return false
		}
	}
	return true
}
