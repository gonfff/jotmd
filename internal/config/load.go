package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

func DefaultPath(lookupEnv func(string) (string, bool)) (string, error) {
	if xdg, ok := lookupEnv("XDG_CONFIG_HOME"); ok && xdg != "" {
		return filepath.Join(xdg, "jotmd", "config.toml"), nil
	}
	home, ok := lookupEnv("HOME")
	if !ok || home == "" {
		return "", errors.New("home directory is not set")
	}
	return filepath.Join(home, ".config", "jotmd", "config.toml"), nil
}

func KeymapPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "keybindings.toml")
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	contents, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var partial Partial
	metadata, err := toml.Decode(string(contents), &partial)
	if err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	if unknown := metadata.Undecoded(); len(unknown) != 0 {
		return Config{}, fmt.Errorf("unknown config field: %s", unknown[0])
	}
	return Merge(cfg, partial), nil
}

func Finalize(cfg Config) (Config, error) {
	var err error
	cfg, err = expandHomeInConfig(cfg)
	if err != nil {
		return Config{}, err
	}
	if err := Validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func expandHomeInConfig(cfg Config) (Config, error) {
	notesDir, err := expandHome(cfg.NotesDir)
	if err != nil {
		return Config{}, err
	}
	cfg.NotesDir = notesDir
	return cfg, nil
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand home directory: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}
