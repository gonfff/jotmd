package config

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

//go:embed assets/config.toml assets/keybindings.toml
var assets embed.FS

func InitAt(configPath string) error {
	return initAt(configPath, "")
}

func InitAtWithNotesDir(configPath, notesDir string) error {
	if notesDir == "" {
		return errors.New("notes directory is not set")
	}
	return initAt(configPath, notesDir)
}

func initAt(configPath, notesDir string) (returnErr error) {
	dir := filepath.Dir(configPath)
	files := []struct {
		name  string
		asset string
	}{
		{name: "keybindings.toml", asset: "assets/keybindings.toml"},
		{name: "config.toml", asset: "assets/config.toml"},
	}
	for _, file := range files {
		if _, err := os.Lstat(filepath.Join(dir, file.name)); err == nil {
			return fmt.Errorf("%s already exists", filepath.Join(dir, file.name))
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("stat %s: %w", filepath.Join(dir, file.name), err)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	created := make([]string, 0, len(files))
	defer func() {
		if returnErr != nil {
			for _, path := range created {
				_ = os.Remove(path)
			}
		}
	}()
	for _, file := range files {
		contents, err := assets.ReadFile(file.asset)
		if err != nil {
			return fmt.Errorf("read bundled %s: %w", file.name, err)
		}
		if file.name == "config.toml" && notesDir != "" {
			var setting bytes.Buffer
			if err := toml.NewEncoder(&setting).Encode(struct {
				NotesDir string `toml:"notes_dir"`
			}{NotesDir: notesDir}); err != nil {
				return fmt.Errorf("encode notes directory: %w", err)
			}
			contents = append(append(contents, '\n'), setting.Bytes()...)
		}
		path := filepath.Join(dir, file.name)
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		created = append(created, path)
		if _, err := out.Write(contents); err != nil {
			_ = out.Close()
			return fmt.Errorf("write %s: %w", path, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("close %s: %w", path, err)
		}
	}
	return nil
}
