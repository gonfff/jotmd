//go:build darwin || linux

package notes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteRevisionPermanentlyDeletesMatchingNote(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "current", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteRevision(context.Background(), "note.md", document.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "note.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted note stat error = %v, want not exist", err)
	}
	assertNoStagingDirectories(t, root)
}

func TestDeleteRevisionMismatchPreservesNote(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "current", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	stale := revisionBytes([]byte("stale"))

	err = store.DeleteRevision(context.Background(), "note.md", stale)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("DeleteRevision() error = %v, want %v", err, ErrRevisionConflict)
	}
	if content, readErr := os.ReadFile(filepath.Join(root, "note.md")); readErr != nil || string(content) != "current" {
		t.Fatalf("preserved content = %q, error = %v", content, readErr)
	}
	assertNoStagingDirectories(t, root)
}

func TestDeleteRevisionRejectsMissingDirectoryAndNonMarkdownTargets(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"directory.md"}, []treeFile{{path: "note.txt", content: "text", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := revisionBytes([]byte("current"))

	if err := store.DeleteRevision(context.Background(), "missing.md", expected); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("DeleteRevision(missing) error = %v, want %v", err, ErrRevisionConflict)
	}
	for _, path := range []RelPath{"directory.md", "note.txt", "../outside.md"} {
		if err := store.DeleteRevision(context.Background(), path, expected); err == nil {
			t.Errorf("DeleteRevision(%q) error = nil", path)
		}
	}
	assertNoStagingDirectories(t, root)
}
