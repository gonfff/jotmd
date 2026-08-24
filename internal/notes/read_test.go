package notes

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestParseRevisionAcceptsOnlySHA256Tokens(t *testing.T) {
	valid := "sha256:" + strings.Repeat("A", 64)
	got, err := ParseRevision(valid)
	if err != nil {
		t.Fatal(err)
	}
	if want := Revision("sha256:" + strings.Repeat("a", 64)); got != want {
		t.Fatalf("ParseRevision(%q) = %q, want %q", valid, got, want)
	}

	for _, value := range []string{"", "abc", "sha256:abc", "sha256:" + strings.Repeat("z", 64)} {
		if _, err := ParseRevision(value); !errors.Is(err, ErrInvalidRevision) {
			t.Errorf("ParseRevision(%q) error = %v, want %v", value, err, ErrInvalidRevision)
		}
	}
}

func TestRevisionBytesMatchesReadRevision(t *testing.T) {
	const emptyRevision = Revision("sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	if got := revisionBytes(nil); got != emptyRevision {
		t.Fatalf("revisionBytes(nil) = %q, want %q", got, emptyRevision)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if document.Revision != emptyRevision {
		t.Fatalf("Read() revision = %q, want %q", document.Revision, emptyRevision)
	}
}

func TestReadReturnsUnmodifiedContentAndContentRevision(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	content := []byte("before\xff\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Content, content) {
		t.Errorf("Read() content = %v, want %v", first.Content, content)
	}
	if first.Revision != second.Revision {
		t.Errorf("Read() revision = %q then %q, want stable revision", first.Revision, second.Revision)
	}

	changed := []byte("after\n")
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Read(context.Background(), "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision == first.Revision {
		t.Error("Read() revision did not change after content changed")
	}
}

func TestReadRejectsFIFOWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(root, "note.md"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := store.Read(context.Background(), "note.md"); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Read() FIFO error = nil, want rejection")
		}
	case <-time.After(time.Second):
		t.Error("Read() FIFO blocked")
	}
}

func TestReadRejectsInternalAndExternalSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTree(t, root, []string{"inside"}, []treeFile{{path: "inside/note.md", content: "inside", mode: 0o600}})
	writeTree(t, outside, nil, []treeFile{{path: "outside.md", content: "outside", mode: 0o600}})
	makeSymlink(t, "inside/note.md", filepath.Join(root, "inside-link.md"))
	makeSymlink(t, "inside", filepath.Join(root, "inside-directory-link"))
	makeSymlink(t, filepath.Join(outside, "outside.md"), filepath.Join(root, "outside-link.md"))
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []RelPath{"inside-link.md", "inside-directory-link/note.md", "outside-link.md"} {
		t.Run(string(path), func(t *testing.T) {
			if _, err := store.Read(context.Background(), path); err == nil {
				t.Errorf("Read(%q) error = nil, want rejection", path)
			}
		})
	}
}

func TestReadLargeFileAtConfiguredDefaultPreviewLimit(t *testing.T) {
	root := t.TempDir()
	limit := DefaultReadMaxBytes
	writeTree(t, root, nil, []treeFile{
		{path: "at-limit.md", content: string(bytes.Repeat([]byte("a"), int(limit))), mode: 0o600},
		{path: "over-limit.md", content: string(bytes.Repeat([]byte("b"), int(limit+1))), mode: 0o600},
	})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetReadMaxBytes(limit); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), "at-limit.md"); err != nil {
		t.Fatalf("Read() at configured limit error = %v", err)
	}
	if document, err := store.Read(context.Background(), "over-limit.md"); err == nil {
		t.Error("Read() over configured limit error = nil, want rejection")
	} else if len(document.Content) != 0 {
		t.Errorf("Read() over configured limit returned %d bytes", len(document.Content))
	} else {
		var tooLarge *TooLargeError
		if !errors.As(err, &tooLarge) || tooLarge.Size != limit+1 || tooLarge.Limit != limit {
			t.Errorf("Read() over configured limit error = %v, want size %d and limit %d", err, limit+1, limit)
		}
	}
}

func TestReadRejectsNonRegularFilesAndCanceledContext(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("note"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Read(context.Background(), "directory"); err == nil {
		t.Error("Read(directory) error = nil, want rejection")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Read(ctx, "note.md"); !errors.Is(err, context.Canceled) {
		t.Errorf("Read() error = %v, want %v", err, context.Canceled)
	}
}
