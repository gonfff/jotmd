package notes

import (
	"context"
	"path/filepath"
	"time"

	"github.com/gonfff/jotmd/internal/fswatch"
)

type Change struct {
	Paths    []RelPath
	Overflow bool
	Err      error
}

func (s *Store) Watch(ctx context.Context, snapshot Snapshot, debounce time.Duration) (<-chan Change, error) {
	root, err := descriptorPath(s.rootFD)
	if err != nil {
		return nil, err
	}
	directories := []string{root}
	for _, entry := range snapshot.Entries {
		if entry.Kind == KindDirectory {
			directories = append(directories, filepath.Join(root, filepath.FromSlash(string(entry.Path))))
		}
	}
	changes, err := fswatch.Watch(ctx, root, directories, debounce)
	if err != nil {
		return nil, err
	}
	return noteChanges(ctx, changes), nil
}

func noteChanges(ctx context.Context, source <-chan fswatch.Change) <-chan Change {
	changes := make(chan Change)
	go func() {
		defer close(changes)
		for change := range source {
			paths := make([]RelPath, len(change.Paths))
			for index, path := range change.Paths {
				paths[index] = RelPath(path)
			}
			select {
			case changes <- Change{Paths: paths, Overflow: change.Overflow, Err: change.Err}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return changes
}
