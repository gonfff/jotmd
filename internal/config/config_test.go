package config_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/notes"
)

func TestDefaults(t *testing.T) {
	want := config.Config{
		NotesDir:         "~/vault",
		Editor:           nil,
		Theme:            "jotmd",
		TreeWidth:        20,
		NoColor:          false,
		ShowHidden:       false,
		Ignore:           []string{".obsidian", ".git"},
		Sort:             "name",
		DirectoriesFirst: true,
		StatusBar:        true,
		Watch:            true,
		Preview: config.Preview{
			Wrap:        true,
			MaxBytes:    2 * 1024 * 1024,
			RenderStyle: "quiet",
		},
	}

	if got := config.Defaults(); !equalConfig(got, want) {
		t.Errorf("Defaults() = %#v, want %#v", got, want)
	}
}

func TestDefaultsUsesNotesDefaultReadMaxBytes(t *testing.T) {
	if got := config.Defaults().Preview.MaxBytes; got != notes.DefaultReadMaxBytes {
		t.Fatalf("Defaults() Preview.MaxBytes = %d, want notes default %d", got, notes.DefaultReadMaxBytes)
	}
}

func TestTreeWidthLoadsValidatesAndDumps(t *testing.T) {
	path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "tree_width = 45\n")
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.TreeWidth != 45 {
		t.Fatalf("Load() TreeWidth = %d, want 45", got.TreeWidth)
	}

	var output bytes.Buffer
	if err := config.Dump(&output, got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "tree_width = 45") {
		t.Fatalf("Dump() = %q, want tree_width", output.String())
	}

	for _, value := range []int{0, 101} {
		invalid := config.Defaults()
		invalid.TreeWidth = value
		if err := config.Validate(invalid); err == nil || !strings.Contains(err.Error(), "tree_width") {
			t.Errorf("Validate(TreeWidth=%d) error = %v, want tree_width error", value, err)
		}
	}
}

func TestPreviewRenderStyleLoadsValidatesAndDumps(t *testing.T) {
	if got := config.Defaults().Preview.RenderStyle; got != "quiet" {
		t.Fatalf("Defaults() Preview.RenderStyle = %q, want quiet", got)
	}
	path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "[preview]\nrender_style = \"surface\"\n")
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Preview.RenderStyle != "surface" {
		t.Fatalf("Load() Preview.RenderStyle = %q, want surface", got.Preview.RenderStyle)
	}
	var output bytes.Buffer
	if err := config.Dump(&output, got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `render_style = "surface"`) {
		t.Fatalf("Dump() = %q, want render_style", output.String())
	}
	got.Preview.RenderStyle = "unknown"
	if err := config.Validate(got); err == nil || !strings.Contains(err.Error(), "preview.render_style") {
		t.Fatalf("Validate() error = %v, want preview.render_style error", err)
	}
}

func TestWatchDefaultsLoadsMergesEnvironmentAndDumps(t *testing.T) {
	defaults := config.Defaults()
	if !defaults.Watch {
		t.Fatal("Defaults() Watch = false, want true")
	}
	if defaults.Preview.MaxBytes != 2*1024*1024 {
		t.Fatalf("Defaults() Preview.MaxBytes = %d, want 2097152", defaults.Preview.MaxBytes)
	}

	path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "watch = false\n")
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Watch {
		t.Error("Load() Watch = true, want false")
	}

	enabled := true
	merged := config.Merge(loaded, config.Partial{Watch: &enabled})
	if !merged.Watch {
		t.Error("Merge() Watch = false, want true")
	}

	got, err := config.ApplyEnv(merged, lookup(map[string]string{"JOTMD_WATCH": "false"}))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := config.Dump(&output, got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "watch = false") || !strings.Contains(output.String(), "max_bytes = 2097152") {
		t.Errorf("Dump() = %q, want effective watch and preview limit", output.String())
	}
}

func TestLoadMergeAndDumpEditorArguments(t *testing.T) {
	path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), `editor = ["nvim", "--cmd", "set spell"]
`)
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"nvim", "--cmd", "set spell"}
	if strings.Join(got.Editor, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Load() Editor = %#v, want %#v", got.Editor, want)
	}

	overlay := []string{"vim", "-f"}
	got = config.Merge(got, config.Partial{Editor: &overlay})
	overlay[0] = "changed"
	if strings.Join(got.Editor, "\x00") != "vim\x00-f" {
		t.Fatalf("Merge() Editor = %#v, want copied arguments", got.Editor)
	}

	var output bytes.Buffer
	if err := config.Dump(&output, got); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := config.Load(writeFile(t, filepath.Join(t.TempDir(), "dump.toml"), output.String()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(roundTrip.Editor, "\x00") != "vim\x00-f" {
		t.Fatalf("Load(Dump()) Editor = %#v, want %#v", roundTrip.Editor, got.Editor)
	}
}

func TestFinalizeRejectsInvalidEditorConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name   string
		editor string
	}{
		{name: "empty command", editor: `[]`},
		{name: "empty executable", editor: `["", "--wait"]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "editor = "+tt.editor+"\n")
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := config.Finalize(cfg); err == nil || !strings.Contains(err.Error(), "editor") || !strings.Contains(err.Error(), "non-empty") {
				t.Fatalf("Finalize() error = %v, want non-empty editor error", err)
			}
		})
	}
}

func TestValidateRejectsDirectoriesFirstFalse(t *testing.T) {
	cfg := config.Defaults()
	cfg.DirectoriesFirst = false
	if err := config.Validate(cfg); err == nil || !strings.Contains(err.Error(), "directories_first") {
		t.Fatalf("Validate() error = %v, want directories_first error", err)
	}
}

func TestApplyEnvOverridesConfiguredValues(t *testing.T) {
	configured := config.Config{
		NotesDir:         "/from-toml",
		Editor:           nil,
		Theme:            "toml",
		TreeWidth:        32,
		ShowHidden:       false,
		Ignore:           []string{".obsidian", ".git"},
		Sort:             "name",
		DirectoriesFirst: true,
		StatusBar:        true,
		Preview: config.Preview{
			Wrap:        true,
			MaxBytes:    10 * 1024 * 1024,
			RenderStyle: "quiet",
		},
	}

	got, err := config.ApplyEnv(configured, lookup(map[string]string{
		"JOTMD_NOTES_DIR": "/from-environment",
		"JOTMD_THEME":     "environment",
		"NO_COLOR":        "1",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if got.NotesDir != "/from-environment" {
		t.Errorf("ApplyEnv() NotesDir = %q, want environment value", got.NotesDir)
	}
	if got.Theme != "environment" {
		t.Errorf("ApplyEnv() Theme = %q, want environment value", got.Theme)
	}
	if !got.NoColor {
		t.Error("ApplyEnv() NoColor = false, want true when NO_COLOR is set")
	}
}

func TestApplyEnvIgnoresEmptyNoColor(t *testing.T) {
	got, err := config.ApplyEnv(config.Defaults(), lookup(map[string]string{"NO_COLOR": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if got.NoColor {
		t.Error("ApplyEnv() NoColor = true, want false for empty NO_COLOR")
	}
}

func TestApplyEnvParsesImplementedSettings(t *testing.T) {
	got, err := config.ApplyEnv(config.Defaults(), lookup(map[string]string{
		"JOTMD_SHOW_HIDDEN":          "true",
		"JOTMD_STATUS_BAR":           "false",
		"JOTMD_PREVIEW_WRAP":         "false",
		"JOTMD_PREVIEW_MAX_BYTES":    "4096",
		"JOTMD_PREVIEW_RENDER_STYLE": "surface",
		"JOTMD_IGNORE":               "tmp:vendor",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !got.ShowHidden || got.StatusBar || got.Preview.Wrap || got.Preview.MaxBytes != 4096 || got.Preview.RenderStyle != "surface" || strings.Join(got.Ignore, ",") != "tmp,vendor" {
		t.Errorf("ApplyEnv() = %#v, want parsed environment settings", got)
	}
}

func TestApplyEnvIdentifiesInvalidSourceField(t *testing.T) {
	_, err := config.ApplyEnv(config.Defaults(), lookup(map[string]string{"JOTMD_SHOW_HIDDEN": "sometimes"}))
	if err == nil || !strings.Contains(err.Error(), "JOTMD_SHOW_HIDDEN") {
		t.Fatalf("ApplyEnv() error = %v, want source field", err)
	}
}

func TestApplyEnvIdentifiesSemanticallyInvalidSourceField(t *testing.T) {
	for name, value := range map[string]string{"JOTMD_PREVIEW_MAX_BYTES": "0", "JOTMD_PREVIEW_RENDER_STYLE": "unknown"} {
		_, err := config.ApplyEnv(config.Defaults(), lookup(map[string]string{name: value}))
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("ApplyEnv(%s) error = %v, want source field", name, err)
		}
	}
}

func TestDefaultPath(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		want      string
		wantError string
	}{
		{
			name: "uses XDG configuration home",
			env:  map[string]string{"XDG_CONFIG_HOME": "/tmp/xdg", "HOME": "/tmp/home"},
			want: "/tmp/xdg/jotmd/config.toml",
		},
		{
			name: "falls back to home configuration directory",
			env:  map[string]string{"HOME": "/tmp/home"},
			want: "/tmp/home/.config/jotmd/config.toml",
		},
		{
			name:      "requires a home directory when XDG is absent",
			wantError: "home",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.DefaultPath(lookup(tt.env))
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("DefaultPath() error = %v, want containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("DefaultPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	configPath := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "notes_dir = \"~/work-notes\"\ntheme = \"night\"\nshow_hidden = true\nignore = [\"vendor\"]\nsort = \"name\"\ndirectories_first = false\nstatus_bar = false\n\n[preview]\nwrap = false\nmax_bytes = 1024\n")

	tests := []struct {
		name      string
		path      string
		want      config.Config
		wantError string
	}{
		{
			name: "merges TOML onto defaults without normalizing paths",
			path: configPath,
			want: config.Config{
				NotesDir:         "~/work-notes",
				Editor:           nil,
				Theme:            "night",
				TreeWidth:        20,
				ShowHidden:       true,
				Ignore:           []string{"vendor"},
				Sort:             "name",
				DirectoriesFirst: false,
				StatusBar:        false,
				Watch:            true,
				Preview: config.Preview{
					Wrap:        false,
					MaxBytes:    1024,
					RenderStyle: "quiet",
				},
			},
		},
		{
			name: "absent default file keeps defaults",
			path: filepath.Join(t.TempDir(), "missing.toml"),
			want: config.Defaults(),
		},
		{
			name:      "rejects unknown TOML field",
			path:      writeFile(t, filepath.Join(t.TempDir(), "unknown.toml"), "unknown = true\n"),
			wantError: "unknown",
		},
		{
			name: "keeps non-leading tilde literal",
			path: writeFile(t, filepath.Join(t.TempDir(), "literal-tilde.toml"), "notes_dir = \"notes/~/literal\"\n"),
			want: config.Config{
				NotesDir:         "notes/~/literal",
				Editor:           nil,
				Theme:            "jotmd",
				TreeWidth:        20,
				ShowHidden:       false,
				Ignore:           []string{".obsidian", ".git"},
				Sort:             "name",
				DirectoriesFirst: true,
				StatusBar:        true,
				Watch:            true,
				Preview: config.Preview{
					Wrap:        true,
					MaxBytes:    2 * 1024 * 1024,
					RenderStyle: "quiet",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.Load(tt.path)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("Load() error = %v, want containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !equalConfig(got, tt.want) {
				t.Errorf("Load() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFinalizeExpandsOnlyInitialTilde(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", homeDir)

	for _, tt := range []struct {
		name string
		path string
		want string
	}{
		{
			name: "initial tilde",
			path: "~/work-notes",
			want: filepath.Join(homeDir, "work-notes"),
		},
		{
			name: "literal tilde",
			path: "notes/~/literal",
			want: "notes/~/literal",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := config.Finalize(defaultConfig(tt.path))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.NotesDir != tt.want {
				t.Errorf("Finalize() NotesDir = %q, want %q", cfg.NotesDir, tt.want)
			}
		})
	}
}

func TestMergeLetsNotesDirOverrideTOML(t *testing.T) {
	base := defaultConfig("/from-toml")
	notesDir := "/from-flag"

	got := config.Merge(base, config.Partial{NotesDir: &notesDir})
	if got.NotesDir != notesDir {
		t.Errorf("Merge() NotesDir = %q, want %q", got.NotesDir, notesDir)
	}
}

func TestMergeCopiesThemeColorOverrides(t *testing.T) {
	colors := map[string]string{"accent": "#FFFFFF"}
	got := config.Merge(config.Defaults(), config.Partial{ThemeColors: &colors})
	colors["accent"] = "#000000"

	if got.ThemeColors["accent"] != "#FFFFFF" {
		t.Errorf("Merge() ThemeColors = %#v, want copied override", got.ThemeColors)
	}
}

func TestLoadAndDumpThemeColorOverrides(t *testing.T) {
	path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), "theme_colors = { accent = \"#FFFFFF\" }\n")
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ThemeColors["accent"] != "#FFFFFF" {
		t.Fatalf("Load() ThemeColors = %#v, want accent override", got.ThemeColors)
	}

	var output bytes.Buffer
	if err := config.Dump(&output, got); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := config.Load(writeFile(t, filepath.Join(t.TempDir(), "roundtrip.toml"), output.String()))
	if err != nil {
		t.Fatal(err)
	}
	if roundTrip.ThemeColors["accent"] != "#FFFFFF" {
		t.Errorf("Load(Dump()) ThemeColors = %#v, want accent override", roundTrip.ThemeColors)
	}
}

func TestDumpProducesLoadableTOML(t *testing.T) {
	cfg := defaultConfig("/tmp/notes")
	var output bytes.Buffer
	if err := config.Dump(&output, cfg); err != nil {
		t.Fatal(err)
	}

	path := writeFile(t, filepath.Join(t.TempDir(), "config.toml"), output.String())
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !equalConfig(got, cfg) {
		t.Errorf("Load(Dump()) = %#v, want %#v", got, cfg)
	}
}

func TestLoadRejectsInvalidFiles(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantError string
	}{
		{
			name:      "invalid TOML",
			path:      writeFile(t, filepath.Join(t.TempDir(), "invalid.toml"), "notes_dir = [\n"),
			wantError: "decode config",
		},
		{
			name:      "directory instead of a file",
			path:      t.TempDir(),
			wantError: "read config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := config.Load(tt.path); err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("Load() error = %v, want containing %q", err, tt.wantError)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
	}{
		{
			name: "empty notes directory",
			cfg:  defaultConfig(""),
		},
		{
			name: "empty theme",
			cfg: config.Config{
				NotesDir:         "/notes",
				Editor:           nil,
				Theme:            "",
				TreeWidth:        32,
				ShowHidden:       false,
				Ignore:           []string{".obsidian", ".git"},
				Sort:             "name",
				DirectoriesFirst: true,
				StatusBar:        true,
				Preview: config.Preview{
					Wrap:        true,
					MaxBytes:    10 * 1024 * 1024,
					RenderStyle: "quiet",
				},
			},
		},
		{
			name: "unsupported sort",
			cfg: config.Config{
				NotesDir:         "/notes",
				Editor:           nil,
				Theme:            "jotmd",
				TreeWidth:        32,
				ShowHidden:       false,
				Ignore:           []string{".obsidian", ".git"},
				Sort:             "modified",
				DirectoriesFirst: true,
				StatusBar:        true,
				Preview: config.Preview{
					Wrap:        true,
					MaxBytes:    2 * 1024 * 1024,
					RenderStyle: "quiet",
				},
			},
		},
		{
			name: "non-positive preview limit",
			cfg: config.Config{
				NotesDir:         "/notes",
				Editor:           nil,
				Theme:            "jotmd",
				TreeWidth:        32,
				ShowHidden:       false,
				Ignore:           []string{".obsidian", ".git"},
				Sort:             "name",
				DirectoriesFirst: true,
				StatusBar:        true,
				Preview: config.Preview{
					Wrap:        true,
					MaxBytes:    0,
					RenderStyle: "quiet",
				},
			},
		},
		{
			name: "preview limit above safe maximum",
			cfg: config.Config{
				NotesDir: "/notes", Editor: nil, Theme: "jotmd", TreeWidth: 32, Ignore: []string{".obsidian", ".git"}, Sort: "name", DirectoriesFirst: true, StatusBar: true,
				Preview: config.Preview{Wrap: true, MaxBytes: 64*1024*1024 + 1, RenderStyle: "quiet"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := config.Validate(tt.cfg); err == nil {
				t.Error("Validate() error = nil, want error")
			}
		})
	}
}

func TestDumpReturnsWriterError(t *testing.T) {
	errWriter := errorWriter{}
	if err := config.Dump(errWriter, defaultConfig("/notes")); !errors.Is(err, errWrite) {
		t.Errorf("Dump() error = %v, want %v", err, errWrite)
	}
}

func defaultConfig(notesDir string) config.Config {
	return config.Config{
		NotesDir:         notesDir,
		Editor:           nil,
		Theme:            "jotmd",
		TreeWidth:        20,
		NoColor:          false,
		ShowHidden:       false,
		Ignore:           []string{".obsidian", ".git"},
		Sort:             "name",
		DirectoriesFirst: true,
		StatusBar:        true,
		Watch:            true,
		Preview: config.Preview{
			Wrap:        true,
			MaxBytes:    2 * 1024 * 1024,
			RenderStyle: "quiet",
		},
	}
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func equalConfig(got, want config.Config) bool {
	if got.NotesDir != want.NotesDir || got.Theme != want.Theme || got.TreeWidth != want.TreeWidth || got.NoColor != want.NoColor || got.ShowHidden != want.ShowHidden || got.Sort != want.Sort || got.DirectoriesFirst != want.DirectoriesFirst || got.StatusBar != want.StatusBar || got.Watch != want.Watch || got.Preview != want.Preview {
		return false
	}
	return strings.Join(got.Ignore, "\x00") == strings.Join(want.Ignore, "\x00") && strings.Join(got.Editor, "\x00") == strings.Join(want.Editor, "\x00")
}

var errWrite = errors.New("write failed")

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) {
	return 0, errWrite
}
