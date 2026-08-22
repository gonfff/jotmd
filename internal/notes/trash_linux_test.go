//go:build linux

package notes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestTrashUsesFreeDesktopHomeTrashWithUniqueNames(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "notes")
	data := filepath.Join(base, "data")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", data)
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	for index, content := range []string{"first", "second"} {
		path := filepath.Join(root, "note.md")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := store.Trash(context.Background(), "note.md", identityFor(t, store, "note.md")); err != nil {
			t.Fatal(err)
		}
		name := "note.md"
		if index == 1 {
			name = "note.md.1"
		}
		trashed, err := os.ReadFile(filepath.Join(data, "Trash", "files", name))
		if err != nil || string(trashed) != content {
			t.Errorf("trashed %q content = %q, error = %v", name, trashed, err)
		}
		info, err := os.ReadFile(filepath.Join(data, "Trash", "info", name+".trashinfo"))
		if err != nil {
			t.Fatal(err)
		}
		if want := "Path=" + filepath.ToSlash(path); !strings.Contains(string(info), want+"\n") {
			t.Errorf("trash info = %q, want encoded original %q", info, want)
		}
	}
}

func TestTrashRejectsChangedAndMissingSourceBeforeMove(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "notes")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := identityFor(t, store, "note.md")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := store.Trash(context.Background(), "note.md", expected); !errors.Is(err, ErrConflict) {
		t.Errorf("Trash() replaced source error = %v, want %v", err, ErrConflict)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "replacement" {
		t.Errorf("replacement content = %q, error = %v", content, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := store.Trash(context.Background(), "note.md", expected); !errors.Is(err, ErrConflict) {
		t.Errorf("Trash() missing source error = %v, want %v", err, ErrConflict)
	}
}

func TestTrashRefusesCrossFilesystemWithoutDeletingOriginal(t *testing.T) {
	data := "/dev/shm"
	dataInfo, err := os.Stat(data)
	if err != nil || !dataInfo.IsDir() {
		t.Skip("/dev/shm is unavailable")
	}
	root := t.TempDir()
	if same, err := sameFilesystem(root, data); err != nil || same {
		t.Skip("test paths are not on distinct filesystems")
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir()+"-unused")
	crossData, err := os.MkdirTemp(data, "jotmd-trash-test-")
	if err != nil {
		t.Skipf("cannot create cross-filesystem data directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(crossData) })
	t.Setenv("XDG_DATA_HOME", crossData)
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Trash(context.Background(), "note.md", identityFor(t, store, "note.md"))
	if !errors.Is(err, ErrTrashCrossFilesystem) {
		t.Errorf("Trash() error = %v, want %v", err, ErrTrashCrossFilesystem)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "keep" {
		t.Errorf("original content = %q, error = %v", content, err)
	}
}

func TestTrashCleansAttemptMetadataWhenMoveDoesNotCommit(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "notes")
	data := filepath.Join(base, "data")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", data)
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := identityFor(t, store, "note.md")
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })

	if err := store.Trash(context.Background(), "note.md", expected); err == nil {
		t.Fatal("Trash() error = nil, want move failure")
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "keep" {
		t.Errorf("original content = %q, error = %v", content, err)
	}
	infoEntries, err := os.ReadDir(filepath.Join(data, "Trash", "info"))
	if err != nil {
		t.Fatal(err)
	}
	if len(infoEntries) != 0 {
		t.Errorf("partial trash metadata = %#v, want empty", infoEntries)
	}
	filesEntries, err := os.ReadDir(filepath.Join(data, "Trash", "files"))
	if err != nil {
		t.Fatal(err)
	}
	if len(filesEntries) != 0 {
		t.Errorf("partial trash files = %#v, want empty", filesEntries)
	}
}

func sameFilesystem(left, right string) (bool, error) {
	var leftStat, rightStat unix.Stat_t
	if err := unix.Stat(left, &leftStat); err != nil {
		return false, err
	}
	if err := unix.Stat(right, &rightStat); err != nil {
		return false, err
	}
	return leftStat.Dev == rightStat.Dev, nil
}
