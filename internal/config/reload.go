package config

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gonfff/jotmd/internal/fswatch"
)

type Resolved struct {
	Config Config
	Keymap Keymap
}

type ReloadResult struct {
	Resolved Resolved
	Err      error
}

type Reloader struct {
	path      string
	lookupEnv func(string) (string, bool)
	overrides Partial
	actions   []Action
}

func NewReloader(path string, lookupEnv func(string) (string, bool), overrides Partial, actions []Action) Reloader {
	return Reloader{path: path, lookupEnv: lookupEnv, overrides: overrides, actions: append([]Action(nil), actions...)}
}

func (r Reloader) Load() (Resolved, error) {
	cfg, err := Load(r.path)
	if err != nil {
		return Resolved{}, err
	}
	cfg, err = ApplyEnv(cfg, r.lookupEnv)
	if err != nil {
		return Resolved{}, err
	}
	cfg, err = Finalize(Merge(cfg, r.overrides))
	if err != nil {
		return Resolved{}, err
	}
	keymap, err := LoadKeymap(KeymapPath(r.path), r.actions)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{Config: cfg, Keymap: keymap}, nil
}

func (r Reloader) Watch(ctx context.Context, debounce time.Duration) (<-chan ReloadResult, error) {
	dir, err := filepath.Abs(filepath.Dir(r.path))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	changes, err := fswatch.Watch(ctx, dir, []string{dir}, debounce)
	if err != nil {
		return nil, err
	}
	configName := filepath.Base(r.path)
	keymapName := filepath.Base(KeymapPath(r.path))
	results := make(chan ReloadResult)
	go func() {
		defer close(results)
		for change := range changes {
			if change.Err != nil {
				if !sendReload(ctx, results, ReloadResult{Err: change.Err}) {
					return
				}
				continue
			}
			if !changedFile(change.Paths, configName) && !changedFile(change.Paths, keymapName) {
				continue
			}
			resolved, err := r.Load()
			if !sendReload(ctx, results, ReloadResult{Resolved: resolved, Err: err}) {
				return
			}
		}
	}()
	return results, nil
}

func changedFile(paths []string, name string) bool {
	for _, path := range paths {
		if path == name {
			return true
		}
	}
	return false
}

func sendReload(ctx context.Context, results chan<- ReloadResult, result ReloadResult) bool {
	select {
	case results <- result:
		return true
	case <-ctx.Done():
		return false
	}
}
