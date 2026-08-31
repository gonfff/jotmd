package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/theme"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type dataErrorReader struct{}

func (dataErrorReader) Read(buffer []byte) (int, error) {
	return copy(buffer, "notes\n"), errors.New("read failed")
}

func setExistingEditor(t *testing.T) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", executable)
}

func TestRun(t *testing.T) {
	original := buildInfo
	buildInfo = func() (*debug.BuildInfo, bool) { return nil, false }
	t.Cleanup(func() { buildInfo = original })

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "prints help",
			args:       []string{"--help"},
			wantCode:   0,
			wantStdout: rootUsage,
		},
		{
			name:       "prints version",
			args:       []string{"--version"},
			wantCode:   0,
			wantStdout: "jotmd dev\n",
		},
		{
			name:       "rejects unknown flag",
			args:       []string{"--unknown"},
			wantCode:   2,
			wantStdout: rootUsage,
			wantStderr: "flag provided but not defined: -unknown\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			gotCode := Run(context.Background(), tt.args, strings.NewReader(""), &stdout, &stderr)

			if gotCode != tt.wantCode {
				t.Errorf("Run() code = %d, want %d", gotCode, tt.wantCode)
			}
			if stdout.String() != tt.wantStdout {
				t.Errorf("Run() stdout = %q, want %q", stdout.String(), tt.wantStdout)
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("Run() stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestWheelFilterDropsOnlySameDirectionWithinFrame(t *testing.T) {
	now := time.Unix(1, 0)
	filter := wheelFilter(func() time.Time { return now })
	down := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown})
	up := tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp})

	if got := filter(nil, down); got != down {
		t.Fatalf("first down = %#v, want unchanged", got)
	}
	if got := filter(nil, down); got != nil {
		t.Fatalf("second down = %#v, want nil", got)
	}
	if got := filter(nil, up); got != up {
		t.Fatalf("direction-changing up = %#v, want unchanged", got)
	}
	now = now.Add(time.Second / 60)
	if got := filter(nil, up); got != up {
		t.Fatalf("next-frame up = %#v, want unchanged", got)
	}

	key := tea.KeyPressMsg(tea.Key{Code: 'j'})
	if got := filter(nil, key); got != key {
		t.Fatalf("key = %#v, want unchanged", got)
	}
}

func TestRunEditorFlagOverridesConfigAndRejectsMalformedCommand(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	configHome := t.TempDir()
	writeConfig(t, filepath.Join(configHome, "jotmd", "config.toml"), `editor = ["configured-editor-that-does-not-exist"]
`)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	var stdout, stderr bytes.Buffer
	flagValue := executable + ` --from-cli "two words"`
	if code := Run(context.Background(), []string{"--editor", flagValue, "--dump-config"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", code, stderr.String())
	}
	for _, want := range []string{executable, `"--from-cli"`, `"two words"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"--editor", executable + ` "unfinished`, "--dump-config"}, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "quote") {
		t.Fatalf("Run() = (%d, %q), want malformed editor error", code, stderr.String())
	}
}

func TestRunReturnsErrorWhenWritingOutputFails(t *testing.T) {
	gotCode := Run(context.Background(), []string{"--version"}, strings.NewReader(""), failWriter{}, &bytes.Buffer{})

	if gotCode != 1 {
		t.Errorf("Run() code = %d, want 1", gotCode)
	}
}

func TestRunReturnsErrorWhenWritingUsageFails(t *testing.T) {
	gotCode := Run(context.Background(), []string{"--help"}, strings.NewReader(""), failWriter{}, &bytes.Buffer{})

	if gotCode != 1 {
		t.Errorf("Run() code = %d, want 1", gotCode)
	}
}

func TestRunValidatesThemeAndNotesRootBeforeStartingTUI(t *testing.T) {
	setExistingEditor(t)
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "theme", args: []string{"--theme", "unknown"}, want: "unknown theme"},
		{name: "notes root", args: []string{"--notes-dir", filepath.Join(t.TempDir(), "missing")}, want: "stat notes root"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			gotCode := Run(context.Background(), test.args, strings.NewReader("\n"), &stdout, &stderr)
			if gotCode != 1 {
				t.Errorf("Run() code = %d, want 1", gotCode)
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Errorf("Run() stderr = %q, want %q", stderr.String(), test.want)
			}
		})
	}
}

func TestRunDumpConfigAppliesNotesDirFlag(t *testing.T) {
	setExistingEditor(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--notes-dir", "/project-memory", "--dump-config"}, strings.NewReader(""), &stdout, &stderr)

	if gotCode != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", gotCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "notes_dir = \"/project-memory\"") {
		t.Errorf("Run() stdout = %q, want notes directory from CLI", stdout.String())
	}
}

func TestRunDumpConfigAppliesEveryConfigurationFlag(t *testing.T) {
	setExistingEditor(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{
		"--theme-colors", `{ accent = "#FFFFFF" }`,
		"--tree-width", "45",
		"--no-color=false",
		"--show-hidden=true",
		"--show-agent-memory=true",
		"--ignore", strings.Join([]string{"tmp", "vendor"}, string(filepath.ListSeparator)),
		"--sort", "name",
		"--directories-first=true",
		"--status-bar=false",
		"--watch=false",
		"--preview-wrap=false",
		"--preview-max-bytes", "4096",
		"--preview-render-style", "surface",
		"--dump-config",
	}, strings.NewReader(""), &stdout, &stderr)

	if gotCode != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", gotCode, stderr.String())
	}
	path := filepath.Join(t.TempDir(), "dump.toml")
	writeConfig(t, path, stdout.String())
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ThemeColors["accent"] != "#FFFFFF" || got.TreeWidth != 45 || got.NoColor || !got.ShowHidden || !got.ShowAgentMemory ||
		!reflect.DeepEqual(got.Ignore, []string{"tmp", "vendor"}) || got.Sort != "name" || !got.DirectoriesFirst || got.StatusBar || got.Watch ||
		got.Preview.Wrap || got.Preview.MaxBytes != 4096 || got.Preview.RenderStyle != "surface" {
		t.Errorf("Run() dumped config = %#v, want CLI settings", got)
	}
}

func TestRunDumpConfigEmptyIgnoreClearsConfiguredNames(t *testing.T) {
	setExistingEditor(t)
	configHome := t.TempDir()
	writeConfig(t, filepath.Join(configHome, "jotmd", "config.toml"), `ignore = ["configured"]`)
	t.Setenv("XDG_CONFIG_HOME", configHome)

	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--ignore=", "--dump-config"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", code, stderr.String())
	}
	path := filepath.Join(t.TempDir(), "dump.toml")
	writeConfig(t, path, stdout.String())
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Ignore) != 0 {
		t.Fatalf("Run() dumped Ignore = %#v, want empty", got.Ignore)
	}
}

func TestRunInitConfigCreatesTemplatesBeforeStartingTUI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--init-config"}, strings.NewReader(""), &stdout, &stderr)
	if gotCode != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", gotCode, stderr.String())
	}
	for _, name := range []string{"config.toml", "keybindings.toml"} {
		if _, err := os.Stat(filepath.Join(home, ".config", "jotmd", name)); err != nil {
			t.Errorf("%s was not created: %v", name, err)
		}
	}
	contents, err := os.ReadFile(filepath.Join(home, ".config", "jotmd", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# notes_dir = \"~/vault\"",
		"# editor = [\"nvim\"]",
		"# theme = \"jotmd\"",
		"# theme_colors = { accent = \"#CBA6F7\" }",
		"# tree_width = 20",
		"# no_color = false",
		"# show_hidden = false",
		"# show_agent_memory = false",
		"# ignore = [\".obsidian\", \".git\"]",
		"# sort = \"name\"",
		"# directories_first = true",
		"# status_bar = true",
		"# watch = true",
		"# [preview]",
		"# wrap = true",
		"# max_bytes = 2097152",
	} {
		if !strings.Contains(string(contents), want) {
			t.Errorf("config template lacks %q:\n%s", want, contents)
		}
	}
	keybindings, err := os.ReadFile(filepath.Join(home, ".config", "jotmd", "keybindings.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(keybindings), `"view.agent_memory" = ["a"]`) {
		t.Errorf("keymap template lacks agent memory binding:\n%s", keybindings)
	}
}

func TestRunInitConfigUsesXDGConfigHome(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--init-config"}, strings.NewReader(""), &stdout, &stderr)
	if gotCode != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", gotCode, stderr.String())
	}
	for _, name := range []string{"config.toml", "keybindings.toml"} {
		if _, err := os.Stat(filepath.Join(configHome, "jotmd", name)); err != nil {
			t.Errorf("%s was not created: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(configHome, "jotmd", "themes")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("--init-config created themes without --init-themes: %v", err)
	}
}

func TestSetupFirstRunCreatesSiblingStoreAndPersistsOnlyItsPath(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "jotmd", "config.toml")
	var output bytes.Buffer

	notesDir, created, err := setupFirstRun(configPath, strings.NewReader("\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	wantNotesDir := filepath.Join(filepath.Dir(configPath), "vault")
	if !created || notesDir != wantNotesDir {
		t.Fatalf("setupFirstRun() = (%q, %v), want (%q, true)", notesDir, created, wantNotesDir)
	}
	if !strings.Contains(output.String(), wantNotesDir) {
		t.Errorf("prompt = %q, want default path", output.String())
	}
	if info, err := os.Stat(notesDir); err != nil || !info.IsDir() {
		t.Fatalf("notes directory = (%v, %v), want directory", info, err)
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.NotesDir != notesDir {
		t.Fatalf("persisted notes_dir = %q, want %q", loaded.NotesDir, notesDir)
	}
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "# theme = \"jotmd\"") || strings.Contains(string(contents), "\ntheme = \"jotmd\"") {
		t.Fatalf("config template activated unrelated settings:\n%s", contents)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(configPath), "themes")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first run exported themes without --init-themes: %v", err)
	}
}

func TestSetupFirstRunRejectsEOFWithoutCreatingFiles(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "jotmd", "config.toml")
	if _, _, err := setupFirstRun(configPath, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("setupFirstRun() EOF error = nil")
	}
	for _, path := range []string{configPath, config.KeymapPath(configPath), filepath.Join(filepath.Dir(configPath), "vault")} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("EOF created %s: %v", path, err)
		}
	}
}

func TestSetupFirstRunRejectsTokenReturnedWithReadError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "jotmd", "config.toml")
	if _, _, err := setupFirstRun(configPath, dataErrorReader{}, &bytes.Buffer{}); err == nil {
		t.Fatal("setupFirstRun() read error = nil")
	}
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read error created config: %v", err)
	}
}

func TestSetupFirstRunDoesNotRequireThemesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "jotmd")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "themes"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "config.toml")
	if _, _, err := setupFirstRun(configPath, strings.NewReader("\n"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("setup did not create config: %v", err)
	}
}

func TestRunDumpKeysDoesNotRequireNotesRoot(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--dump-keys"}, strings.NewReader(""), &stdout, &stderr)
	if gotCode != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", gotCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\"tree.down\" = [\"j\", \"down\"]") {
		t.Errorf("Run() stdout = %q, want default tree.down binding", stdout.String())
	}
}

func TestRunThemeCommandsDoNotRequireNotesRoot(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"--init-themes"}, want: "created"},
		{args: []string{"--list-themes"}, want: "jotmd\n"},
		{args: []string{"--dump-theme", "jotmd"}, want: "name = \"jotmd\""},
	} {
		var stdout, stderr bytes.Buffer
		if gotCode := Run(context.Background(), test.args, strings.NewReader(""), &stdout, &stderr); gotCode != 0 {
			t.Fatalf("Run(%v) code = %d, want 0; stderr = %s", test.args, gotCode, stderr.String())
		}
		if !strings.Contains(stdout.String(), test.want) {
			t.Errorf("Run(%v) stdout = %q, want %q", test.args, stdout.String(), test.want)
		}
	}
	if _, err := os.Stat(filepath.Join(configHome, "jotmd", "themes", "jotmd.toml")); err != nil {
		t.Errorf("bundled jotmd theme was not created: %v", err)
	}
}

func TestRunThemeInspectionCommandsDoNotWriteThemes(t *testing.T) {
	setExistingEditor(t)
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"--list-themes"}, want: "jotmd\n"},
		{args: []string{"--dump-theme", "jotmd"}, want: "name = \"jotmd\""},
		{args: []string{"--dump-config"}, want: "theme = \"jotmd\""},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			configHome := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", configHome)
			var stdout, stderr bytes.Buffer
			if gotCode := Run(context.Background(), test.args, strings.NewReader(""), &stdout, &stderr); gotCode != 0 {
				t.Fatalf("Run(%v) code = %d, want 0; stderr = %s", test.args, gotCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Errorf("Run(%v) stdout = %q, want %q", test.args, stdout.String(), test.want)
			}
			if _, err := os.Stat(filepath.Join(configHome, "jotmd", "themes")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("theme inspection created themes directory: %v", err)
			}
		})
	}
}

func TestRunListsBuiltinThemesAndResolvesAuto(t *testing.T) {
	setExistingEditor(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--list-themes"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("Run(--list-themes) code = %d, want 0; stderr = %s", code, stderr.String())
	}
	want := strings.Join(theme.Names(), "\n") + "\n"
	if stdout.String() != want {
		t.Errorf("Run(--list-themes) = %q, want %q", stdout.String(), want)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"--theme", "auto", "--dump-config"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("Run(--theme auto --dump-config) code = %d, want 0; stderr = %s", code, stderr.String())
	}
}

func TestResolveThemeNoColorClearsBuiltinPalette(t *testing.T) {
	cfg := config.Defaults()
	cfg.NoColor = true
	th, err := resolveTheme(t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	palette := reflect.ValueOf(th.Palette)
	for index := range palette.NumField() {
		if got := palette.Field(index).String(); got != "" {
			t.Errorf("resolveTheme() Palette.%s = %q, want empty", palette.Type().Field(index).Name, got)
		}
	}
}

func TestResolveThemeLoadsBuiltinNameFromUserThemeDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := theme.EnsureBundled(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "jotmd.toml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	contents = []byte(strings.Replace(string(contents), "[palette]\n", "[palette]\nbackground = \"#000000\"\n", 1))
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := resolveTheme(dir, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if got.Palette.Background != "#000000" {
		t.Fatalf("resolveTheme() Background = %q, want value from theme file", got.Palette.Background)
	}
}

func TestResolveAutoThemeLoadsJotMDFromUserThemeDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := theme.EnsureBundled(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "jotmd.toml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	contents = []byte(strings.Replace(string(contents), "[palette]\n", "[palette]\nbackground = \"#000000\"\n", 1))
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.Theme = "auto"
	got, err := resolveTheme(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Palette.Background != "#000000" {
		t.Fatalf("resolveTheme(auto) Background = %q, want user jotmd theme", got.Palette.Background)
	}
}

func TestLoadThemesUsesUserThemeDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := theme.EnsureBundled(dir); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(dir, "jotmd.toml"))
	if err != nil {
		t.Fatal(err)
	}
	contents = []byte(strings.Replace(string(contents), "name = \"jotmd\"", "name = \"custom\"", 1))
	if err := os.WriteFile(filepath.Join(dir, "custom.toml"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
	themes, err := loadThemes(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := append(theme.Names(), "custom")
	slices.Sort(wantNames)
	gotNames := make([]string, len(themes))
	for index := range themes {
		gotNames[index] = themes[index].Name
	}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("theme names = %q, want %q", gotNames, wantNames)
	}
}

func TestRunRejectsEmptyDumpTheme(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--dump-theme="}, strings.NewReader(""), &stdout, &stderr)
	if gotCode != 1 || !strings.Contains(stderr.String(), "theme name must not be empty") {
		t.Fatalf("Run() = (%d, %q), want empty theme name error", gotCode, stderr.String())
	}
}

func TestRunDumpConfigValidatesTheme(t *testing.T) {
	setExistingEditor(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--theme", "unknown", "--dump-config"}, strings.NewReader(""), &stdout, &stderr)
	if gotCode != 1 || !strings.Contains(stderr.String(), "unknown theme") {
		t.Fatalf("Run() = (%d, %q), want unknown theme error", gotCode, stderr.String())
	}
}

func TestRunDumpConfigDoesNotLoadMalformedKeybindings(t *testing.T) {
	setExistingEditor(t)
	configHome := t.TempDir()
	writeConfig(t, filepath.Join(configHome, "jotmd", "keybindings.toml"), "[bindings\n")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--dump-config"}, strings.NewReader(""), &stdout, &stderr)
	if gotCode != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", gotCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "notes_dir") {
		t.Errorf("Run() stdout = %q, want dumped configuration", stdout.String())
	}
}

func TestRunDumpConfigUsesCLIOverEnvironmentAndTOML(t *testing.T) {
	setExistingEditor(t)
	configHome := t.TempDir()
	writeConfig(t, filepath.Join(configHome, "jotmd", "config.toml"), "notes_dir = \"/from-toml\"\ntheme = \"jotmd\"\n")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("JOTMD_NOTES_DIR", "/from-environment")
	t.Setenv("JOTMD_THEME", "jotmd")
	t.Setenv("NO_COLOR", "1")

	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--notes-dir", "/from-cli", "--theme", "jotmd", "--dump-config"}, strings.NewReader(""), &stdout, &stderr)

	if gotCode != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", gotCode, stderr.String())
	}
	for _, want := range []string{
		"notes_dir = \"/from-cli\"",
		"theme = \"jotmd\"",
		"no_color = true",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunDumpConfigUsesEnvironmentOverTOML(t *testing.T) {
	setExistingEditor(t)
	configHome := t.TempDir()
	writeConfig(t, filepath.Join(configHome, "jotmd", "config.toml"), "notes_dir = \"/from-toml\"\ntheme = \"jotmd\"\n")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("JOTMD_NOTES_DIR", "/from-environment")
	t.Setenv("JOTMD_THEME", "jotmd")

	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--dump-config"}, strings.NewReader(""), &stdout, &stderr)

	if gotCode != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", gotCode, stderr.String())
	}
	for _, want := range []string{
		"notes_dir = \"/from-environment\"",
		"theme = \"jotmd\"",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("Run() stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunConfigCheckValidatesExplicitSiblingFiles(t *testing.T) {
	setExistingEditor(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	writeConfig(t, configPath, "theme = \"custom\"\nwatch = false\n")
	writeConfig(t, filepath.Join(dir, "keybindings.toml"), "[bindings]\n\"tree.down\" = [\"ctrl+j\"]\n")
	bundled, err := theme.Builtin("jotmd")
	if err != nil {
		t.Fatal(err)
	}
	bundled.Name = "custom"
	var themeTOML bytes.Buffer
	if err := theme.Dump(&themeTOML, bundled); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, filepath.Join(dir, "themes", "custom.toml"), themeTOML.String())

	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"config", "check", configPath}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("Run(config check) code = %d, want 0; stderr = %s", code, stderr.String())
	}
	if want := "configuration is valid: " + configPath + "\n"; stdout.String() != want {
		t.Errorf("Run(config check) stdout = %q, want %q", stdout.String(), want)
	}
}

func TestRunConfigCheckRejectsMissingConfiguredEditor(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	missing := filepath.Join(t.TempDir(), "editor-that-does-not-exist")
	writeConfig(t, configPath, "editor = [\""+missing+"\"]\n")

	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"config", "check", configPath}, strings.NewReader(""), &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), missing) {
		t.Fatalf("Run(config check) = (%d, %q), want missing editor error", code, stderr.String())
	}
}

func TestRunConfigCheckRejectsInvalidEnvironmentAndArguments(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("JOTMD_WATCH", "sometimes")
	for _, args := range [][]string{{"config", "check"}, {"config", "check", "one", "two"}} {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), args, strings.NewReader(""), &stdout, &stderr)
		if code == 0 {
			t.Fatalf("Run(%v) code = 0, want error", args)
		}
	}
}

func TestRunConfigCheckUsesDefaultPathAndRejectsInvalidSiblingKeymap(t *testing.T) {
	setExistingEditor(t)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"config", "check"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("Run(config check) code = %d, want 0; stderr = %s", code, stderr.String())
	}

	configPath := filepath.Join(configHome, "jotmd", "config.toml")
	writeConfig(t, configPath, "theme = \"jotmd\"\n")
	writeConfig(t, filepath.Join(configHome, "jotmd", "keybindings.toml"), "[bindings\n")
	stdout.Reset()
	stderr.Reset()
	if code := Run(context.Background(), []string{"config", "check", configPath}, strings.NewReader(""), &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "decode keybindings") {
		t.Fatalf("Run(config check malformed keymap) = (%d, %q), want decode error", code, stderr.String())
	}
}

func TestRunDumpConfigAppliesEnvironmentBeforeExpandingTOMLNotesDir(t *testing.T) {
	setExistingEditor(t)
	configHome := t.TempDir()
	writeConfig(t, filepath.Join(configHome, "jotmd", "config.toml"), "notes_dir = \"~\"\n")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("HOME", "")
	t.Setenv("JOTMD_NOTES_DIR", "/from-environment")

	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--dump-config"}, strings.NewReader(""), &stdout, &stderr)

	if gotCode != 0 {
		t.Fatalf("Run() code = %d, want 0; stderr = %s", gotCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "notes_dir = \"/from-environment\"") {
		t.Errorf("Run() stdout = %q, want environment notes directory", stdout.String())
	}
}

func TestRunReportsConfigurationErrors(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*testing.T)
		args      []string
		wantError string
	}{
		{
			name: "missing home directory",
			setup: func(t *testing.T) {
				t.Setenv("XDG_CONFIG_HOME", "")
				t.Setenv("HOME", "")
			},
			args:      []string{"--dump-config"},
			wantError: "home directory",
		},
		{
			name: "invalid TOML",
			setup: func(t *testing.T) {
				configHome := t.TempDir()
				configPath := filepath.Join(configHome, "jotmd", "config.toml")
				if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configPath, []byte("unknown = true\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("XDG_CONFIG_HOME", configHome)
			},
			args:      []string{"--dump-config"},
			wantError: "unknown config field",
		},
		{
			name: "empty notes directory flag",
			setup: func(t *testing.T) {
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			},
			args:      []string{"--notes-dir=", "--dump-config"},
			wantError: "notes_dir must not be empty",
		},
		{
			name: "invalid preview max bytes environment setting",
			setup: func(t *testing.T) {
				t.Setenv("XDG_CONFIG_HOME", t.TempDir())
				t.Setenv("JOTMD_PREVIEW_MAX_BYTES", "0")
			},
			args:      []string{"--dump-config"},
			wantError: "JOTMD_PREVIEW_MAX_BYTES",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			var stdout, stderr bytes.Buffer
			gotCode := Run(context.Background(), tt.args, strings.NewReader(""), &stdout, &stderr)
			if gotCode != 1 {
				t.Errorf("Run() code = %d, want 1", gotCode)
			}
			if !strings.Contains(stderr.String(), tt.wantError) {
				t.Errorf("Run() stderr = %q, want containing %q", stderr.String(), tt.wantError)
			}
		})
	}
}

func TestVersionUsesVCSRevisionDuringDevelopment(t *testing.T) {
	original := buildInfo
	buildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "abc123"}}}, true
	}
	t.Cleanup(func() { buildInfo = original })

	if got := Version(); got != "dev (abc123)" {
		t.Errorf("Version() = %q, want %q", got, "dev (abc123)")
	}
}

func TestRunUsesReleaseVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	gotCode := Run(context.Background(), []string{"--version"}, strings.NewReader(""), &stdout, &stderr, "v1.2.3")

	if gotCode != 0 {
		t.Fatalf("Run() code = %d, want 0", gotCode)
	}
	if got := stdout.String(); got != "jotmd v1.2.3\n" {
		t.Errorf("Run() stdout = %q, want %q", got, "jotmd v1.2.3\n")
	}
}

func writeConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
