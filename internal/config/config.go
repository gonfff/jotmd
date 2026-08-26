package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gonfff/jotmd/internal/notes"
)

type Preview struct {
	Wrap        bool
	MaxBytes    int64
	RenderStyle string
}

type Config struct {
	NotesDir         string
	Editor           []string
	Theme            string
	ThemeColors      map[string]string
	TreeWidth        int
	NoColor          bool
	ShowHidden       bool
	Ignore           []string
	Sort             string
	DirectoriesFirst bool
	StatusBar        bool
	Watch            bool
	Preview          Preview
}

type Partial struct {
	NotesDir         *string            `toml:"notes_dir"`
	Editor           *[]string          `toml:"editor"`
	Theme            *string            `toml:"theme"`
	ThemeColors      *map[string]string `toml:"theme_colors"`
	TreeWidth        *int               `toml:"tree_width"`
	NoColor          *bool              `toml:"no_color"`
	ShowHidden       *bool              `toml:"show_hidden"`
	Ignore           *[]string          `toml:"ignore"`
	Sort             *string            `toml:"sort"`
	DirectoriesFirst *bool              `toml:"directories_first"`
	StatusBar        *bool              `toml:"status_bar"`
	Watch            *bool              `toml:"watch"`
	Preview          *partialPreview    `toml:"preview"`
}

type partialPreview struct {
	Wrap        *bool   `toml:"wrap"`
	MaxBytes    *int64  `toml:"max_bytes"`
	RenderStyle *string `toml:"render_style"`
}

func Defaults() Config {
	return Config{
		NotesDir:         "~/vault",
		Editor:           nil,
		Theme:            "jotmd",
		ThemeColors:      nil,
		TreeWidth:        20,
		NoColor:          false,
		ShowHidden:       false,
		Ignore:           []string{".obsidian", ".git"},
		Sort:             "name",
		DirectoriesFirst: true,
		StatusBar:        true,
		Watch:            true,
		Preview: Preview{
			Wrap:        true,
			MaxBytes:    notes.DefaultReadMaxBytes,
			RenderStyle: "quiet",
		},
	}
}

func ApplyEnv(base Config, lookupEnv func(string) (string, bool)) (Config, error) {
	var overlay Partial
	if notesDir, ok := lookupEnv("JOTMD_NOTES_DIR"); ok {
		overlay.NotesDir = &notesDir
	}
	if theme, ok := lookupEnv("JOTMD_THEME"); ok {
		overlay.Theme = &theme
	}
	if value, ok := lookupEnv("JOTMD_NO_COLOR"); ok {
		parsed, err := envBool("JOTMD_NO_COLOR", value)
		if err != nil {
			return Config{}, err
		}
		overlay.NoColor = &parsed
	}
	if value, ok := lookupEnv("JOTMD_SHOW_HIDDEN"); ok {
		parsed, err := envBool("JOTMD_SHOW_HIDDEN", value)
		if err != nil {
			return Config{}, err
		}
		overlay.ShowHidden = &parsed
	}
	if value, ok := lookupEnv("JOTMD_IGNORE"); ok {
		ignore := strings.Split(value, string(filepath.ListSeparator))
		overlay.Ignore = &ignore
	}
	if value, ok := lookupEnv("JOTMD_STATUS_BAR"); ok {
		parsed, err := envBool("JOTMD_STATUS_BAR", value)
		if err != nil {
			return Config{}, err
		}
		overlay.StatusBar = &parsed
	}
	if value, ok := lookupEnv("JOTMD_WATCH"); ok {
		parsed, err := envBool("JOTMD_WATCH", value)
		if err != nil {
			return Config{}, err
		}
		overlay.Watch = &parsed
	}
	if value, ok := lookupEnv("JOTMD_PREVIEW_WRAP"); ok {
		parsed, err := envBool("JOTMD_PREVIEW_WRAP", value)
		if err != nil {
			return Config{}, err
		}
		overlay.Preview = &partialPreview{Wrap: &parsed}
	}
	if value, ok := lookupEnv("JOTMD_PREVIEW_MAX_BYTES"); ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("parse JOTMD_PREVIEW_MAX_BYTES: %w", err)
		}
		if parsed < 1 {
			return Config{}, errors.New("JOTMD_PREVIEW_MAX_BYTES must be positive")
		}
		if overlay.Preview == nil {
			overlay.Preview = &partialPreview{}
		}
		overlay.Preview.MaxBytes = &parsed
	}
	if value, ok := lookupEnv("JOTMD_PREVIEW_RENDER_STYLE"); ok {
		if !validRenderStyle(value) {
			return Config{}, errors.New("JOTMD_PREVIEW_RENDER_STYLE must be quiet, surface, or structural")
		}
		if overlay.Preview == nil {
			overlay.Preview = &partialPreview{}
		}
		overlay.Preview.RenderStyle = &value
	}
	base = Merge(base, overlay)
	if noColor, ok := lookupEnv("NO_COLOR"); ok && noColor != "" {
		base.NoColor = true
	}
	return base, nil
}

func envBool(name, value string) (bool, error) {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func Merge(base Config, overlay Partial) Config {
	if overlay.NotesDir != nil {
		base.NotesDir = *overlay.NotesDir
	}
	if overlay.Editor != nil {
		base.Editor = make([]string, len(*overlay.Editor))
		copy(base.Editor, *overlay.Editor)
	}
	if overlay.Theme != nil {
		base.Theme = *overlay.Theme
	}
	if overlay.ThemeColors != nil {
		base.ThemeColors = make(map[string]string, len(*overlay.ThemeColors))
		for name, color := range *overlay.ThemeColors {
			base.ThemeColors[name] = color
		}
	}
	if overlay.TreeWidth != nil {
		base.TreeWidth = *overlay.TreeWidth
	}
	if overlay.NoColor != nil {
		base.NoColor = *overlay.NoColor
	}
	if overlay.ShowHidden != nil {
		base.ShowHidden = *overlay.ShowHidden
	}
	if overlay.Ignore != nil {
		base.Ignore = append([]string(nil), (*overlay.Ignore)...)
	}
	if overlay.Sort != nil {
		base.Sort = *overlay.Sort
	}
	if overlay.DirectoriesFirst != nil {
		base.DirectoriesFirst = *overlay.DirectoriesFirst
	}
	if overlay.StatusBar != nil {
		base.StatusBar = *overlay.StatusBar
	}
	if overlay.Watch != nil {
		base.Watch = *overlay.Watch
	}
	if overlay.Preview != nil {
		if overlay.Preview.Wrap != nil {
			base.Preview.Wrap = *overlay.Preview.Wrap
		}
		if overlay.Preview.MaxBytes != nil {
			base.Preview.MaxBytes = *overlay.Preview.MaxBytes
		}
		if overlay.Preview.RenderStyle != nil {
			base.Preview.RenderStyle = *overlay.Preview.RenderStyle
		}
	}
	return base
}
