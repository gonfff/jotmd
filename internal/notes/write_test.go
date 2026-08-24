//go:build darwin || linux

package notes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesMissingNoteWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.Write(context.Background(), "note.md", []byte("created"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != "note.md" || result.Revision != revisionBytes([]byte("created")) || !result.Created {
		t.Fatalf("Write(create) = %#v", result)
	}
	if _, err := store.Write(context.Background(), "note.md", []byte("replacement"), nil); !errors.Is(err, ErrExists) {
		t.Fatalf("Write(existing) error = %v, want %v", err, ErrExists)
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "created" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
	assertNoStagingDirectories(t, root)
}

func TestWriteUpdatesOnlyMatchingRevision(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "old", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatal(err)
	}

	stale := revisionBytes([]byte("stale"))
	if _, err := store.Write(context.Background(), "note.md", []byte("wrong"), &stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("Write(stale) error = %v, want %v", err, ErrRevisionConflict)
	} else {
		var conflict *RevisionConflictError
		if !errors.As(err, &conflict) || conflict.Expected != stale || conflict.Actual != document.Revision {
			t.Fatalf("Write(stale) conflict = %#v", conflict)
		}
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "old" {
		t.Fatalf("content after conflict = %q, error = %v", content, err)
	}

	result, err := store.Write(context.Background(), "note.md", []byte("new"), &document.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != "note.md" || result.Revision != revisionBytes([]byte("new")) || result.Created {
		t.Fatalf("Write(update) = %#v", result)
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "new" {
		t.Fatalf("updated content = %q, error = %v", content, err)
	}
	assertNoStagingDirectories(t, root)
}

func TestWriteRejectsMissingTargetWithRevision(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := revisionBytes([]byte("old"))

	_, err = store.Write(context.Background(), "missing.md", []byte("new"), &expected)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("Write(missing) error = %v, want %v", err, ErrRevisionConflict)
	}
	var conflict *RevisionConflictError
	if !errors.As(err, &conflict) || conflict.Expected != expected || conflict.Actual != "" {
		t.Fatalf("Write(missing) conflict = %#v", conflict)
	}
	assertNoStagingDirectories(t, root)
}

func TestWriteAcceptsEmptyContentAndRejectsInvalidTargets(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "directory.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.Write(context.Background(), "empty.md", nil, nil); err != nil || result.Revision != revisionBytes(nil) {
		t.Fatalf("Write(empty) = (%#v, %v)", result, err)
	}

	expected := revisionBytes(nil)
	for _, path := range []RelPath{"note.txt", "../outside.md", "missing/note.md", "directory.md"} {
		t.Run(string(path), func(t *testing.T) {
			if _, err := store.Write(context.Background(), path, []byte("content"), &expected); err == nil {
				t.Fatalf("Write(%q) error = nil", path)
			}
		})
	}
	assertNoStagingDirectories(t, root)
}

func TestWriteAllowsOnlyOneWriterForTheSameRevision(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "old", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, content := range [][]byte{[]byte("writer-a"), []byte("writer-b")} {
		content := content
		go func() {
			<-start
			_, err := store.Write(context.Background(), "note.md", content, &document.Revision)
			results <- err
		}()
	}
	close(start)

	successes, conflicts := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			successes++
		case errors.Is(err, ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("Write() error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	content, err := os.ReadFile(filepath.Join(root, "note.md"))
	if err != nil || string(content) != "writer-a" && string(content) != "writer-b" {
		t.Fatalf("final content = %q, error = %v", content, err)
	}
	assertNoStagingDirectories(t, root)
}

func TestWriteCanceledBeforeMutationLeavesExistingNote(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "keep", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Write(ctx, "note.md", []byte("replace"), &document.Revision); !errors.Is(err, context.Canceled) {
		t.Fatalf("Write(canceled) error = %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "keep" {
		t.Fatalf("content = %q, error = %v", content, err)
	}
	assertNoStagingDirectories(t, root)
}
