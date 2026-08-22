package config

import (
	"errors"
	"fmt"
	"io"

	"github.com/BurntSushi/toml"
)

const maxPreviewBytes int64 = 64 * 1024 * 1024

func Validate(cfg Config) error {
	if cfg.NotesDir == "" {
		return errors.New("notes_dir must not be empty")
	}
	if cfg.Editor != nil && (len(cfg.Editor) == 0 || cfg.Editor[0] == "") {
		return errors.New("editor must contain a non-empty executable")
	}
	if cfg.Theme == "" {
		return errors.New("theme must not be empty")
	}
	if cfg.TreeWidth < 1 || cfg.TreeWidth > 100 {
		return errors.New("tree_width must be between 1 and 100")
	}
	if cfg.Sort != "name" {
		return fmt.Errorf("sort must be %q", "name")
	}
	if !cfg.DirectoriesFirst {
		return errors.New("directories_first must be true")
	}
	if cfg.Preview.MaxBytes < 1 {
		return errors.New("preview.max_bytes must be positive")
	}
	if cfg.Preview.MaxBytes > maxPreviewBytes {
		return fmt.Errorf("preview.max_bytes must not exceed %d", maxPreviewBytes)
	}
	if !validRenderStyle(cfg.Preview.RenderStyle) {
		return errors.New("preview.render_style must be quiet, surface, or structural")
	}
	return nil
}

func validRenderStyle(style string) bool {
	return style == "quiet" || style == "surface" || style == "structural"
}

type dumpPreview struct {
	Wrap        bool   `toml:"wrap"`
	MaxBytes    int64  `toml:"max_bytes"`
	RenderStyle string `toml:"render_style"`
}

type dumpConfig struct {
	NotesDir         string            `toml:"notes_dir"`
	Editor           []string          `toml:"editor"`
	Theme            string            `toml:"theme"`
	ThemeColors      map[string]string `toml:"theme_colors,omitempty"`
	TreeWidth        int               `toml:"tree_width"`
	NoColor          bool              `toml:"no_color"`
	ShowHidden       bool              `toml:"show_hidden"`
	Ignore           []string          `toml:"ignore"`
	Sort             string            `toml:"sort"`
	DirectoriesFirst bool              `toml:"directories_first"`
	StatusBar        bool              `toml:"status_bar"`
	Watch            bool              `toml:"watch"`
	Preview          dumpPreview       `toml:"preview"`
}

func Dump(w io.Writer, cfg Config) error {
	encoded := dumpConfig{
		NotesDir:         cfg.NotesDir,
		Editor:           cfg.Editor,
		Theme:            cfg.Theme,
		ThemeColors:      cfg.ThemeColors,
		TreeWidth:        cfg.TreeWidth,
		NoColor:          cfg.NoColor,
		ShowHidden:       cfg.ShowHidden,
		Ignore:           cfg.Ignore,
		Sort:             cfg.Sort,
		DirectoriesFirst: cfg.DirectoriesFirst,
		StatusBar:        cfg.StatusBar,
		Watch:            cfg.Watch,
		Preview: dumpPreview{
			Wrap:        cfg.Preview.Wrap,
			MaxBytes:    cfg.Preview.MaxBytes,
			RenderStyle: cfg.Preview.RenderStyle,
		},
	}
	if err := toml.NewEncoder(w).Encode(encoded); err != nil {
		return fmt.Errorf("dump config: %w", err)
	}
	return nil
}
