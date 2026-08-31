package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gonfff/jotmd/internal/config"
	"github.com/gonfff/jotmd/internal/editor"
	"github.com/gonfff/jotmd/internal/notes"
	"github.com/gonfff/jotmd/internal/theme"
	"github.com/gonfff/jotmd/internal/ui"
)

var buildInfo = debug.ReadBuildInfo

func Version() string {
	info, ok := buildInfo()
	if !ok {
		return "dev"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return fmt.Sprintf("dev (%s)", setting.Value)
		}
	}
	return "dev"
}

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, releaseVersion ...string) int {
	invocation, handled, cliErr := parseCLIInvocation(args)
	if handled {
		if cliErr != nil {
			return writeCLIError(stderr, invocation.jsonOutput, invalidCLIInput("%s", cliErr))
		}
		return runCLI(ctx, invocation, stdin, stdout, stderr)
	}

	flags := flag.NewFlagSet("jotmd", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var outputErr error
	flags.Usage = func() {
		_, outputErr = io.WriteString(stdout, rootUsage)
	}

	version := flags.Bool("version", false, "print version")
	notesDir := flags.String("notes-dir", "", "override notes directory")
	themeName := flags.String("theme", "", "override theme")
	editorFlag := flags.String("editor", "", "override editor command")
	initConfig := flags.Bool("init-config", false, "create config and keybindings templates")
	initThemes := flags.Bool("init-themes", false, "create missing bundled themes")
	listThemes := flags.Bool("list-themes", false, "list available themes")
	dumpTheme := flags.String("dump-theme", "", "print a theme as TOML")
	dumpConfig := flags.Bool("dump-config", false, "print effective configuration")
	dumpKeys := flags.Bool("dump-keys", false, "print effective key bindings")
	if err := flags.Parse(args); err != nil {
		if outputErr != nil {
			return 1
		}
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *version {
		versionString := Version()
		if len(releaseVersion) > 0 && releaseVersion[0] != "" {
			versionString = releaseVersion[0]
		}
		if _, err := fmt.Fprintf(stdout, "jotmd %s\n", versionString); err != nil {
			return 1
		}
		return 0
	}
	if len(flags.Args()) > 0 {
		if len(flags.Args()) < 2 || flags.Arg(0) != "config" || flags.Arg(1) != "check" || len(flags.Args()) > 3 {
			return reportError(stderr, errors.New("usage: jotmd config check [PATH]"))
		}
		path := ""
		if len(flags.Args()) == 3 {
			path = flags.Arg(2)
		} else {
			var err error
			path, err = config.DefaultPath(os.LookupEnv)
			if err != nil {
				return reportError(stderr, err)
			}
		}
		if err := checkConfig(path); err != nil {
			return reportError(stderr, err)
		}
		if _, err := fmt.Fprintf(stdout, "configuration is valid: %s\n", path); err != nil {
			return 1
		}
		return 0
	}
	dumpThemeSet := false
	flags.Visit(func(flag *flag.Flag) {
		dumpThemeSet = dumpThemeSet || flag.Name == "dump-theme"
	})
	if *initConfig {
		path, err := config.DefaultPath(os.LookupEnv)
		if err != nil {
			return reportError(stderr, err)
		}
		if err := config.InitAt(path); err != nil {
			return reportError(stderr, err)
		}
		if _, err := fmt.Fprintf(stdout, "created %s and %s\n", path, config.KeymapPath(path)); err != nil {
			return 1
		}
		return 0
	}

	path, err := config.DefaultPath(os.LookupEnv)
	if err != nil {
		return reportError(stderr, err)
	}
	themesDir := filepath.Join(filepath.Dir(path), "themes")
	if *initThemes {
		if err := theme.EnsureBundled(themesDir); err != nil {
			return reportError(stderr, err)
		}
		if _, err := fmt.Fprintf(stdout, "created missing bundled themes in %s\n", themesDir); err != nil {
			return 1
		}
		return 0
	}
	if *listThemes {
		names, err := theme.List(themesDir)
		if err != nil {
			return reportError(stderr, err)
		}
		for _, name := range names {
			if _, err := fmt.Fprintln(stdout, name); err != nil {
				return 1
			}
		}
		return 0
	}
	if dumpThemeSet {
		if *dumpTheme == "" {
			return reportError(stderr, errors.New("theme name must not be empty"))
		}
		th, err := theme.Load(themesDir, *dumpTheme)
		if err != nil {
			return reportError(stderr, err)
		}
		if err := theme.Dump(stdout, th); err != nil {
			return reportError(stderr, err)
		}
		return 0
	}
	if *dumpKeys {
		keymap, err := config.LoadKeymap(config.KeymapPath(path), ui.ActionRegistry())
		if err != nil {
			return reportError(stderr, err)
		}
		if err := config.DumpKeymap(stdout, keymap); err != nil {
			return reportError(stderr, err)
		}
		return 0
	}
	notesDirSet := false
	flags.Visit(func(flag *flag.Flag) { notesDirSet = notesDirSet || flag.Name == "notes-dir" })
	_, notesDirEnvSet := os.LookupEnv("JOTMD_NOTES_DIR")
	if !*dumpConfig && !notesDirSet && !notesDirEnvSet {
		if _, _, err := setupFirstRun(path, stdin, stdout); err != nil {
			return reportError(stderr, err)
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		return reportError(stderr, err)
	}
	cfg, err = config.ApplyEnv(cfg, os.LookupEnv)
	if err != nil {
		return reportError(stderr, err)
	}
	reloadOverrides := config.Partial{}
	flags.Visit(func(flag *flag.Flag) {
		switch flag.Name {
		case "notes-dir":
			cfg = config.Merge(cfg, config.Partial{NotesDir: notesDir})
			reloadOverrides.NotesDir = notesDir
		case "theme":
			cfg = config.Merge(cfg, config.Partial{Theme: themeName})
			reloadOverrides.Theme = themeName
		}
	})
	if cfg, err = config.Finalize(cfg); err != nil {
		return reportError(stderr, err)
	}
	editorSet := false
	flags.Visit(func(flag *flag.Flag) { editorSet = editorSet || flag.Name == "editor" })
	if editorSet && *editorFlag == "" {
		return reportError(stderr, errors.New("editor command must not be empty"))
	}
	editorCommand, err := editor.Resolve(*editorFlag, cfg.Editor)
	if err != nil {
		return reportError(stderr, err)
	}
	cfg.Editor = append([]string{editorCommand.Executable}, editorCommand.Args...)
	if *dumpConfig {
		if _, err := resolveTheme(themesDir, cfg); err != nil {
			return reportError(stderr, err)
		}
		if err := config.Dump(stdout, cfg); err != nil {
			return reportError(stderr, err)
		}
		return 0
	}
	th, err := resolveTheme(themesDir, cfg)
	if err != nil {
		return reportError(stderr, err)
	}
	themes, err := loadThemes(themesDir)
	if err != nil {
		return reportError(stderr, err)
	}
	keymap, err := config.LoadKeymap(config.KeymapPath(path), ui.ActionRegistry())
	if err != nil {
		return reportError(stderr, err)
	}

	store, err := notes.NewStore(cfg.NotesDir, cfg.Ignore, cfg.ShowHidden)
	if err != nil {
		return reportError(stderr, err)
	}
	if err := store.SetReadMaxBytes(cfg.Preview.MaxBytes); err != nil {
		return reportError(stderr, err)
	}
	model := ui.NewModelWithOptions(store, cfg, th, keymap, themes)
	reloader := config.NewReloader(path, os.LookupEnv, reloadOverrides, ui.ActionRegistry())
	model.SetConfigReloader(&reloader, func(cfg config.Config) (theme.Theme, error) {
		return resolveTheme(themesDir, cfg)
	})
	model.SetEditor(editorCommand)
	model.SetContext(ctx)
	if _, err := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithInput(stdin),
		tea.WithOutput(stdout),
		tea.WithFilter(wheelFilter(time.Now)),
	).Run(); err != nil {
		return reportError(stderr, err)
	}
	return 0
}

func wheelFilter(now func() time.Time) func(tea.Model, tea.Msg) tea.Msg {
	var lastButton tea.MouseButton
	var lastAt time.Time
	return func(_ tea.Model, message tea.Msg) tea.Msg {
		wheel, ok := message.(tea.MouseWheelMsg)
		if !ok {
			return message
		}
		current := now()
		if wheel.Button == lastButton && current.Sub(lastAt) < time.Second/60 {
			return nil
		}
		lastButton, lastAt = wheel.Button, current
		return message
	}
}

func checkConfig(path string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	cfg, err = config.ApplyEnv(cfg, os.LookupEnv)
	if err != nil {
		return err
	}
	if cfg, err = config.Finalize(cfg); err != nil {
		return err
	}
	if _, err := editor.Resolve("", cfg.Editor); err != nil {
		return err
	}
	if _, err := config.LoadKeymap(config.KeymapPath(path), ui.ActionRegistry()); err != nil {
		return err
	}
	_, err = resolveTheme(filepath.Join(filepath.Dir(path), "themes"), cfg)
	return err
}

func resolveTheme(dir string, cfg config.Config) (theme.Theme, error) {
	name := cfg.Theme
	if name == "auto" {
		name = "jotmd"
	}
	th, err := theme.Load(dir, name)
	if err != nil {
		return theme.Theme{}, err
	}
	return theme.Apply(th, cfg.ThemeColors, cfg.NoColor)
}

func loadThemes(dir string) ([]theme.Theme, error) {
	names, err := theme.List(dir)
	if err != nil {
		return nil, err
	}
	themes := make([]theme.Theme, 0, len(names))
	for _, name := range names {
		th, err := theme.Load(dir, name)
		if err != nil {
			return nil, err
		}
		themes = append(themes, th)
	}
	return themes, nil
}

func reportError(w io.Writer, err error) int {
	_, _ = fmt.Fprintf(w, "jotmd: %v\n", err)
	return 1
}

func setupFirstRun(configPath string, input io.Reader, output io.Writer) (string, bool, error) {
	if _, err := os.Lstat(configPath); err == nil {
		return "", false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("stat config %q: %w", configPath, err)
	}

	defaultDir := filepath.Join(filepath.Dir(configPath), "vault")
	if _, err := fmt.Fprintf(output, "Notes directory [%s]: ", defaultDir); err != nil {
		return "", false, fmt.Errorf("write first-run prompt: %w", err)
	}
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", false, fmt.Errorf("read notes directory: %w", err)
		}
		return "", false, errors.New("read notes directory: input ended before a response")
	}
	selected := strings.TrimSpace(scanner.Text())
	if err := scanner.Err(); err != nil {
		return "", false, fmt.Errorf("read notes directory: %w", err)
	}
	if selected == "" {
		selected = defaultDir
	}
	cfg := config.Defaults()
	cfg.NotesDir = selected
	cfg, err := config.Finalize(cfg)
	if err != nil {
		return "", false, err
	}
	notesDir, err := filepath.Abs(cfg.NotesDir)
	if err != nil {
		return "", false, fmt.Errorf("resolve notes directory: %w", err)
	}
	if err := os.MkdirAll(notesDir, 0o700); err != nil {
		return "", false, fmt.Errorf("create notes directory: %w", err)
	}
	if err := config.InitAtWithNotesDir(configPath, notesDir); err != nil {
		return "", false, err
	}
	return notesDir, true, nil
}
