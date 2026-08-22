package fswatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestCollectReportsErrorsAndOverflow(t *testing.T) {
	events := make(chan fsnotify.Event)
	errs := make(chan error, 2)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	changes := Collect(ctx, t.TempDir(), events, errs, time.Millisecond)
	wantErr := errors.New("watch failed")
	errs <- wantErr
	if change := awaitChange(t, changes); !errors.Is(change.Err, wantErr) || change.Overflow {
		t.Fatalf("error change = %#v, want ordinary error", change)
	}
	errs <- fsnotify.ErrEventOverflow
	if change := awaitChange(t, changes); !errors.Is(change.Err, fsnotify.ErrEventOverflow) || !change.Overflow {
		t.Fatalf("overflow change = %#v, want overflow error", change)
	}
}

func TestCollectIgnoresSiblingEventsOutsideRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "notes")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	events := make(chan fsnotify.Event, 2)
	errs := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	changes := Collect(ctx, root, events, errs, time.Millisecond)
	events <- fsnotify.Event{Name: filepath.Join(parent, "sibling.md")}
	events <- fsnotify.Event{Name: filepath.Join(root, "inside.md")}

	change := awaitChange(t, changes)
	if !reflect.DeepEqual(change.Paths, []string{"inside.md"}) {
		t.Fatalf("Collect() paths = %#v, want inside event only", change.Paths)
	}
}

func awaitChange(t *testing.T, changes <-chan Change) Change {
	t.Helper()
	select {
	case change, open := <-changes:
		if !open {
			t.Fatal("Collect() closed before reporting change")
		}
		return change
	case <-time.After(2 * time.Second):
		t.Fatal("Collect() did not report change")
		return Change{}
	}
}
