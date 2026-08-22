package notes

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestWatchReportsNestedFilesystemChange(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"nested"}, nil)
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	changes, err := store.Watch(ctx, snapshot, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "note.md"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case change := <-changes:
		if change.Err != nil || len(change.Paths) == 0 {
			t.Fatalf("Watch() change = %#v, want changed path without error", change)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch() did not report nested create")
	}
}

func TestWatchReportsNestedWriteRenameAndDelete(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"nested"}, []treeFile{{path: "nested/note.md", content: "one", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	changes, err := store.Watch(ctx, snapshot, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(root, "nested", "note.md")
	for _, action := range []struct {
		name string
		do   func() error
		want RelPath
	}{
		{name: "write", do: func() error { return os.WriteFile(note, []byte("two"), 0o600) }, want: "nested/note.md"},
		{name: "rename", do: func() error { return os.Rename(note, filepath.Join(root, "nested", "renamed.md")) }, want: "nested/note.md"},
		{name: "delete", do: func() error { return os.Remove(filepath.Join(root, "nested", "renamed.md")) }, want: "nested/renamed.md"},
	} {
		t.Run(action.name, func(t *testing.T) {
			if err := action.do(); err != nil {
				t.Fatal(err)
			}
			change := awaitChange(t, changes)
			if !containsPath(change.Paths, action.want) {
				t.Fatalf("Watch() paths = %#v, want %q", change.Paths, action.want)
			}
		})
	}
}

func TestWatchCoalescesAndDeduplicatesChanges(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "one", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	debounce := 40 * time.Millisecond
	changes, err := store.Watch(ctx, snapshot, debounce)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "note.md")
	for _, content := range []string{"two", "three", "four"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	change := awaitChange(t, changes)
	if !reflect.DeepEqual(change.Paths, []RelPath{"note.md"}) {
		t.Fatalf("Watch() paths = %#v, want one deduplicated path", change.Paths)
	}
	select {
	case extra := <-changes:
		t.Fatalf("Watch() emitted an uncoalesced extra change: %#v", extra)
	case <-time.After(2 * debounce):
	}
}

func TestWatchNeedsRestartedSnapshotForNewDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	changes, err := store.Watch(ctx, snapshot, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = awaitChange(t, changes)
	if err := os.WriteFile(filepath.Join(root, "new", "before.md"), []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case change := <-changes:
		t.Fatalf("old snapshot unexpectedly watched new directory: %#v", change)
	case <-time.After(80 * time.Millisecond):
	}
	cancel()

	snapshot, err = store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	t.Cleanup(cancel)
	changes, err = store.Watch(ctx, snapshot, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "new", "after.md"), []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	if change := awaitChange(t, changes); !containsPath(change.Paths, "new/after.md") {
		t.Fatalf("restarted Watch() paths = %#v, want new/after.md", change.Paths)
	}
}

func TestWatchCancellationClosesChanges(t *testing.T) {
	store, err := NewStore(t.TempDir(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	changes, err := store.Watch(ctx, snapshot, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case _, open := <-changes:
		if open {
			t.Fatal("Watch() emitted change after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch() did not close after cancellation")
	}
}

func TestWatchRemainsConfinedWhenOpenedRootPathIsReplaced(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "notes")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	openedRoot := filepath.Join(parent, "opened")
	if err := os.Rename(root, openedRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	changes, err := store.Watch(ctx, snapshot, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "outside.md"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case change := <-changes:
		t.Fatalf("Watch() followed replaced root path: %#v", change)
	case <-time.After(50 * time.Millisecond):
	}
	if err := os.WriteFile(filepath.Join(openedRoot, "inside.md"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if change := awaitChange(t, changes); !containsPath(change.Paths, "inside.md") {
		t.Fatalf("Watch() paths = %#v, want confined inside.md", change.Paths)
	}
}

func awaitChange(t *testing.T, changes <-chan Change) Change {
	t.Helper()
	select {
	case change, open := <-changes:
		if !open {
			t.Fatal("Watch() closed before reporting change")
		}
		return change
	case <-time.After(2 * time.Second):
		t.Fatal("Watch() did not report change")
		return Change{}
	}
}

func containsPath(paths []RelPath, want RelPath) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
