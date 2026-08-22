package notes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAbsolutePathStaysInOpenedRootAfterRootPathReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "notes")
	original := filepath.Join(parent, "notes-original")
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "original", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, original); err != nil {
		t.Fatal(err)
	}
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "replacement", mode: 0o600}})

	got, err := store.AbsolutePath("note.md")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(original, "note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("AbsolutePath() = %q, want opened-root path %q", got, want)
	}
}
