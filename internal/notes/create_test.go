package notes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCreateNotePlacesFilesInRequestedDirectoryWithExclusiveNames(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "parent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "name.md"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "name-2.md"), []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	previousUmask := unix.Umask(0o022)
	t.Cleanup(func() { unix.Umask(previousUmask) })
	first, err := store.CreateNote(context.Background(), "", "name")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateNote(context.Background(), "parent", "Nested note")
	if err != nil {
		t.Fatal(err)
	}

	if first.Path != "name-3.md" {
		t.Errorf("CreateNote() root path = %q, want %q", first.Path, "name-3.md")
	}
	if second.Path != "parent/nested-note.md" {
		t.Errorf("CreateNote() parent path = %q, want %q", second.Path, "parent/nested-note.md")
	}
	if len(first.Content) != 0 || len(second.Content) != 0 {
		t.Errorf("CreateNote() content = %q, %q, want empty", first.Content, second.Content)
	}
	info, err := os.Stat(filepath.Join(root, string(first.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o644); got != want {
		t.Errorf("CreateNote() mode = %o, want %o from 0666 under umask 022", got, want)
	}
	content, err := os.ReadFile(filepath.Join(root, "name.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "original" {
		t.Errorf("existing note content = %q, want %q", content, "original")
	}
	if _, err := store.CreateNote(context.Background(), "missing", "note"); err == nil {
		t.Error("CreateNote() in missing parent error = nil, want error")
	}
	if _, err := os.Stat(filepath.Join(root, "missing")); !os.IsNotExist(err) {
		t.Errorf("missing parent stat error = %v, want not exist", err)
	}
}

func TestCloseCreatedNoteReturnsContextualError(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "created-note")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	err = closeCreatedNote(file, "note.md")
	if err == nil || !strings.Contains(err.Error(), `close created note "note.md"`) {
		t.Fatalf("closeCreatedNote() error = %v, want contextual close error", err)
	}
}

func TestCreateStaysInOpenedRootAfterPathReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "notes")
	backup := filepath.Join(parent, "notes-original")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}

	document, err := store.CreateNote(context.Background(), "", "inside")
	if err != nil {
		t.Fatal(err)
	}
	directory, err := store.CreateDirectory(context.Background(), "", "folder")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{string(document.Path), string(directory.Path)} {
		if _, err := os.Stat(filepath.Join(backup, path)); err != nil {
			t.Errorf("opened root %q stat error = %v", path, err)
		}
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Errorf("replacement root %q stat error = %v, want not exist", path, err)
		}
	}
}

func TestCreateDirectoryUsesRequestedParentWithoutCreatingMissingParents(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "parent"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	entry, err := store.CreateDirectory(context.Background(), "parent", "child")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Path != "parent/child" || entry.Name != "child" || entry.Kind != KindDirectory {
		t.Errorf("CreateDirectory() = %#v, want parent/child directory entry", entry)
	}
	if _, err := store.CreateDirectory(context.Background(), "missing", "child"); err == nil {
		t.Error("CreateDirectory() in missing parent error = nil, want error")
	}
	if _, err := os.Stat(filepath.Join(root, "missing")); !os.IsNotExist(err) {
		t.Errorf("missing parent stat error = %v, want not exist", err)
	}
}

func TestCreateNoteConcurrentlyUsesDistinctPaths(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	const writers = 24
	type createdNote struct {
		path    RelPath
		content string
	}
	created := make(chan createdNote, writers)
	errs := make(chan error, writers)
	var group sync.WaitGroup
	for index := range writers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			document, err := store.CreateNote(context.Background(), "", "name")
			if err != nil {
				errs <- err
				return
			}
			content := fmt.Sprintf("writer-%d", index)
			if err := os.WriteFile(filepath.Join(root, string(document.Path)), []byte(content), 0o600); err != nil {
				errs <- err
				return
			}
			created <- createdNote{path: document.Path, content: content}
		}(index)
	}
	group.Wait()
	close(created)
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	seen := make(map[RelPath]struct{}, writers)
	for note := range created {
		if _, exists := seen[note.path]; exists {
			t.Errorf("duplicate created path %q", note.path)
		}
		seen[note.path] = struct{}{}
		got, err := os.ReadFile(filepath.Join(root, string(note.path)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != note.content {
			t.Errorf("created note %q content = %q, want %q", note.path, got, note.content)
		}
	}
	if len(seen) != writers {
		t.Errorf("created %d paths, want %d", len(seen), writers)
	}
}
