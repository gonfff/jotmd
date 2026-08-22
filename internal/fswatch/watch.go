package fswatch

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Change struct {
	Paths    []string
	Overflow bool
	Err      error
}

func Watch(ctx context.Context, root string, directories []string, debounce time.Duration) (<-chan Change, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, directory := range directories {
		if err := watcher.Add(directory); err != nil {
			_ = watcher.Close()
			return nil, err
		}
	}
	changes := Collect(ctx, root, watcher.Events, watcher.Errors, debounce)
	go func() {
		<-ctx.Done()
		_ = watcher.Close()
	}()
	return changes, nil
}

func Collect(ctx context.Context, root string, events <-chan fsnotify.Event, errs <-chan error, debounce time.Duration) <-chan Change {
	if debounce <= 0 {
		debounce = time.Millisecond
	}
	changes := make(chan Change)
	go func() {
		defer close(changes)
		pending := make(map[string]struct{})
		var timer *time.Timer
		var timerC <-chan time.Time
		stopTimer := func() {
			if timer != nil && !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		defer stopTimer()
		send := func(change Change) bool {
			select {
			case changes <- change:
				return true
			case <-ctx.Done():
				return false
			}
		}
		flush := func() bool {
			paths := make([]string, 0, len(pending))
			for path := range pending {
				paths = append(paths, path)
			}
			sort.Strings(paths)
			clear(pending)
			return len(paths) == 0 || send(Change{Paths: paths})
		}
		for {
			select {
			case <-ctx.Done():
				return
			case event, open := <-events:
				if !open {
					return
				}
				relative, err := filepath.Rel(root, event.Name)
				if err != nil || relative == "." || !filepath.IsLocal(relative) {
					continue
				}
				pending[filepath.ToSlash(relative)] = struct{}{}
				if timer == nil {
					timer = time.NewTimer(debounce)
				} else {
					stopTimer()
					timer.Reset(debounce)
				}
				timerC = timer.C
			case err, open := <-errs:
				if !open {
					errs = nil
					continue
				}
				if !send(Change{Overflow: errors.Is(err, fsnotify.ErrEventOverflow), Err: err}) {
					return
				}
			case <-timerC:
				timerC = nil
				if !flush() {
					return
				}
			}
		}
	}()
	return changes
}
