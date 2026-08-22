package config_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/ui"
)

func TestReloaderLoadsOneValidatedConfigurationWithPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, filepath.Join(dir, "config.toml"), "notes_dir = \"/from-file\"\ntheme = \"nord\"\nstatus_bar = false\n")
	writeFile(t, config.KeymapPath(path), "[bindings]\n\"tree.down\" = [\"ctrl+n\"]\n")
	notesDir := "/from-cli"
	themeName := "dracula"
	reloader := config.NewReloader(path, lookup(map[string]string{"JOTMD_NOTES_DIR": "/from-env"}), config.Partial{NotesDir: &notesDir, Theme: &themeName}, ui.ActionRegistry())

	resolved, err := reloader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Config.NotesDir != notesDir || resolved.Config.StatusBar || resolved.Config.Theme != themeName {
		t.Fatalf("Load() config = %#v, want CLI notes/theme and hidden status", resolved.Config)
	}
	if keys := bindingKeys(resolved.Keymap, "tree.down"); strings.Join(keys, ",") != "ctrl+n" {
		t.Fatalf("Load() tree.down = %#v, want ctrl+n", keys)
	}
	if err := os.WriteFile(path, []byte("theme = \"nord\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err = reloader.Load()
	if err != nil || resolved.Config.Theme != themeName {
		t.Fatalf("Load() after file theme change = (%#v, %v), want CLI theme %q", resolved.Config, err, themeName)
	}
}

func TestReloaderRemovalReturnsFileBackedValuesToDefaultsAndKeepsCLIOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, filepath.Join(dir, "config.toml"), "notes_dir = \"/from-file\"\ntheme = \"nord\"\nstatus_bar = false\n")
	notesDir := "/from-cli"
	reloader := config.NewReloader(path, lookup(nil), config.Partial{NotesDir: &notesDir}, ui.ActionRegistry())

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	resolved, err := reloader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Config.NotesDir != notesDir || resolved.Config.Theme != "jotmd" || !resolved.Config.StatusBar {
		t.Fatalf("Load() after removal = %#v, want defaults plus CLI notes", resolved.Config)
	}
}

func TestReloaderWatchReloadsOnlySelectedUserFiles(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, filepath.Join(dir, "config.toml"), "theme = \"jotmd\"\n")
	reloader := config.NewReloader(path, lookup(nil), config.Partial{}, ui.ActionRegistry())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	results, err := reloader.Watch(ctx, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	projectConfig := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, projectConfig, "theme = \"dracula\"\n")
	writeFile(t, filepath.Join(dir, "project.toml"), "theme = \"dracula\"\n")
	select {
	case result := <-results:
		t.Fatalf("Watch() reloaded project-local file: %#v", result)
	case <-time.After(50 * time.Millisecond):
	}

	if err := os.WriteFile(path, []byte("unknown = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalid := awaitReload(t, results)
	if invalid.Err == nil || !strings.Contains(invalid.Err.Error(), "unknown config field") {
		t.Fatalf("Watch() invalid result error = %v, want config diagnostic", invalid.Err)
	}

	if err := os.WriteFile(path, []byte("theme = \"dracula\"\nstatus_bar = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := awaitReload(t, results)
	if valid.Err != nil || valid.Resolved.Config.Theme != "dracula" || valid.Resolved.Config.StatusBar {
		t.Fatalf("Watch() valid result = %#v, want resolved dracula config", valid)
	}

	if err := os.WriteFile(config.KeymapPath(path), []byte("[bindings]\n\"tree.down\" = [\"ctrl+n\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	keymap := awaitReload(t, results)
	if keymap.Err != nil || strings.Join(bindingKeys(keymap.Resolved.Keymap, "tree.down"), ",") != "ctrl+n" {
		t.Fatalf("Watch() keymap result = %#v, want tree.down ctrl+n", keymap)
	}
	if err := os.Remove(config.KeymapPath(path)); err != nil {
		t.Fatal(err)
	}
	defaultKeymap := awaitReload(t, results)
	if defaultKeymap.Err != nil || strings.Join(bindingKeys(defaultKeymap.Resolved.Keymap, "tree.down"), ",") != "j,down" {
		t.Fatalf("Watch() keymap removal result = %#v, want defaults", defaultKeymap)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	removed := awaitReload(t, results)
	if removed.Err != nil || removed.Resolved.Config.Theme != "jotmd" {
		t.Fatalf("Watch() removal result = %#v, want defaults", removed)
	}

	cancel()
	select {
	case _, open := <-results:
		if open {
			t.Fatal("Watch() emitted after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch() did not close after cancellation")
	}
}

func TestReloaderWatchCreatesMissingSelectedParentAndReloadsNewConfig(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing")
	path := filepath.Join(parent, "config.toml")
	reloader := config.NewReloader(path, lookup(nil), config.Partial{}, ui.ActionRegistry())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	results, err := reloader.Watch(ctx, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("created parent mode = %v, want private directory", info.Mode())
	}
	for _, unexpected := range []string{path, config.KeymapPath(path)} {
		if _, err := os.Stat(unexpected); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Watch() created %q: %v", unexpected, err)
		}
	}

	if err := os.WriteFile(path, []byte("theme = \"nord\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved := awaitReload(t, results)
	if resolved.Err != nil || resolved.Resolved.Config.Theme != "nord" {
		t.Fatalf("Watch() new config result = %#v, want nord", resolved)
	}
}

func awaitReload(t *testing.T, results <-chan config.ReloadResult) config.ReloadResult {
	t.Helper()
	select {
	case result, open := <-results:
		if !open {
			t.Fatal("Watch() closed before reload")
		}
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("Watch() did not reload")
		return config.ReloadResult{Err: errors.New("unreachable")}
	}
}

func bindingKeys(keymap config.Keymap, action string) []string {
	for _, binding := range keymap.Bindings {
		if binding.Action == action {
			return binding.Keys
		}
	}
	return nil
}
