package notes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenameMovesNoteAndDirectoryWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"folder"}, []treeFile{
		{path: "note.md", content: "note", mode: 0o600},
		{path: "taken.md", content: "taken", mode: 0o600},
	})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	noteIdentity := identityFor(t, store, "note.md")
	got, err := store.Rename(context.Background(), "note.md", "renamed.md", noteIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if got != "renamed.md" {
		t.Errorf("Rename() path = %q, want %q", got, "renamed.md")
	}
	if content, err := os.ReadFile(filepath.Join(root, "renamed.md")); err != nil || string(content) != "note" {
		t.Errorf("renamed note content = %q, error = %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(root, "note.md")); !os.IsNotExist(err) {
		t.Errorf("old note stat error = %v, want not exist", err)
	}

	folderIdentity := identityFor(t, store, "folder")
	got, err = store.Rename(context.Background(), "folder", "renamed-folder", folderIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if got != "renamed-folder" {
		t.Errorf("Rename() directory path = %q, want %q", got, "renamed-folder")
	}

	if _, err := store.Rename(context.Background(), "renamed.md", "taken.md", identityFor(t, store, "renamed.md")); !errors.Is(err, ErrExists) {
		t.Errorf("Rename() collision error = %v, want %v", err, ErrExists)
	}
	if content, err := os.ReadFile(filepath.Join(root, "taken.md")); err != nil || string(content) != "taken" {
		t.Errorf("collision target content = %q, error = %v", content, err)
	}
}

func TestRenameRejectsInvalidNameAndChangedOrMissingSource(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "old", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := identityFor(t, store, "note.md")

	for _, name := range []string{"", ".", "..", "nested/name.md", "bad\\name.md", "bad\nname.md"} {
		if _, err := store.Rename(context.Background(), "note.md", name, expected); err == nil {
			t.Errorf("Rename() newName %q error = nil, want rejection", name)
		}
	}

	if err := os.Remove(filepath.Join(root, "note.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rename(context.Background(), "note.md", "renamed.md", expected); !errors.Is(err, ErrConflict) {
		t.Errorf("Rename() replaced source error = %v, want %v", err, ErrConflict)
	}
	if err := os.Remove(filepath.Join(root, "note.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rename(context.Background(), "note.md", "renamed.md", expected); !errors.Is(err, ErrConflict) {
		t.Errorf("Rename() missing source error = %v, want %v", err, ErrConflict)
	}
}

func TestMutationsRejectReservedStagingBasenames(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*testing.T, *Store) error
	}{
		{
			name: "CreateDirectory",
			run: func(ctx *testing.T, store *Store) error {
				_, err := store.CreateDirectory(context.Background(), "", ".jotmd-stage-user")
				return err
			},
		},
		{
			name: "Rename",
			run: func(ctx *testing.T, store *Store) error {
				_, err := store.Rename(context.Background(), "note.md", ".jotmd-stage-user.md", identityFor(ctx, store, "note.md"))
				return err
			},
		},
		{
			name: "Copy",
			run: func(ctx *testing.T, store *Store) error {
				_, err := store.Copy(context.Background(), "note.md", ".jotmd-stage-user.md", identityFor(ctx, store, "note.md"))
				return err
			},
		},
		{
			name: "Move",
			run: func(ctx *testing.T, store *Store) error {
				_, err := store.Move(context.Background(), "note.md", ".jotmd-stage-user.md", identityFor(ctx, store, "note.md"))
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, nil, []treeFile{{path: "note.md", content: "note", mode: 0o600}})
			store, err := NewStore(root, nil, true)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.run(t, store); err == nil {
				t.Fatal("reserved basename error = nil, want rejection")
			}
		})
	}
}

func TestRenameUnchangedNameIsIdentityCheckedNoOp(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "keep", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := identityFor(t, store, "note.md")

	got, err := store.Rename(context.Background(), "note.md", "note.md", expected)
	if err != nil || got != "note.md" {
		t.Fatalf("Rename() unchanged = %q, %v; want verified no-op", got, err)
	}
	if _, err := store.Rename(context.Background(), "note.md", "note.md", FileIdentity{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("Rename() unchanged wrong identity error = %v, want %v", err, ErrConflict)
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "keep" {
		t.Fatalf("unchanged content = %q, error = %v", content, err)
	}
}

func TestCopyNoteCopiesContentWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"archive"}, []treeFile{
		{path: "note.md", content: "note", mode: 0o600},
		{path: "archive/taken.md", content: "taken", mode: 0o600},
	})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := identityFor(t, store, "note.md")

	got, err := store.Copy(context.Background(), "note.md", "archive/copied.md", expected)
	if err != nil {
		t.Fatal(err)
	}
	if got != "archive/copied.md" {
		t.Errorf("Copy() path = %q, want archive/copied.md", got)
	}
	for path, want := range map[string]string{"note.md": "note", "archive/copied.md": "note"} {
		content, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(content) != want {
			t.Errorf("%s content = %q, error = %v", path, content, err)
		}
	}
	if _, err := store.Copy(context.Background(), "note.md", "archive/taken.md", expected); !errors.Is(err, ErrExists) {
		t.Errorf("Copy() collision error = %v, want %v", err, ErrExists)
	}
	if content, err := os.ReadFile(filepath.Join(root, "archive", "taken.md")); err != nil || string(content) != "taken" {
		t.Errorf("collision target content = %q, error = %v", content, err)
	}
	for _, destination := range []RelPath{"archive/copied.txt", "../outside.md"} {
		if _, err := store.Copy(context.Background(), "note.md", destination, expected); err == nil {
			t.Errorf("Copy() destination %q error = nil, want rejection", destination)
		}
	}
	if err := os.Remove(filepath.Join(root, "note.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Copy(context.Background(), "note.md", "archive/stale.md", expected); !errors.Is(err, ErrConflict) {
		t.Errorf("Copy() replaced source error = %v, want %v", err, ErrConflict)
	}
	assertNoStagingDirectories(t, filepath.Join(root, "archive"))
}

func TestMoveNoteMovesAcrossDirectoriesWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"archive"}, []treeFile{
		{path: "note.md", content: "note", mode: 0o600},
		{path: "keep.md", content: "keep", mode: 0o600},
	})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.Move(context.Background(), "note.md", "archive/moved.md", identityFor(t, store, "note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "archive/moved.md" {
		t.Errorf("Move() path = %q, want archive/moved.md", got)
	}
	if _, err := os.Stat(filepath.Join(root, "note.md")); !os.IsNotExist(err) {
		t.Errorf("source stat error = %v, want not exist", err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "archive", "moved.md")); err != nil || string(content) != "note" {
		t.Errorf("moved content = %q, error = %v", content, err)
	}
	if _, err := store.Move(context.Background(), "keep.md", "archive/moved.md", identityFor(t, store, "keep.md")); !errors.Is(err, ErrExists) {
		t.Errorf("Move() collision error = %v, want %v", err, ErrExists)
	}
	if content, err := os.ReadFile(filepath.Join(root, "keep.md")); err != nil || string(content) != "keep" {
		t.Errorf("collision source content = %q, error = %v", content, err)
	}
}

func TestCopyDirectoryCopiesTreeWithoutOverwritingOrNestingInItself(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"docs/guide", "archive"}, []treeFile{
		{path: "docs/guide/note.md", content: "note", mode: 0o600},
		{path: "docs/asset.txt", content: "asset", mode: 0o600},
	})
	makeSymlink(t, "guide/note.md", filepath.Join(root, "docs", "link.md"))
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := identityFor(t, store, "docs")

	got, err := store.Copy(context.Background(), "docs", "archive/docs", expected)
	if err != nil {
		t.Fatal(err)
	}
	if got != "archive/docs" {
		t.Errorf("Copy() path = %q, want archive/docs", got)
	}
	for path, want := range map[string]string{
		"docs/guide/note.md":         "note",
		"archive/docs/guide/note.md": "note",
		"archive/docs/asset.txt":     "asset",
	} {
		content, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(content) != want {
			t.Errorf("%s content = %q, error = %v", path, content, err)
		}
	}
	if target, err := os.Readlink(filepath.Join(root, "archive", "docs", "link.md")); err != nil || target != "guide/note.md" {
		t.Errorf("copied symlink target = %q, error = %v", target, err)
	}
	if _, err := store.Copy(context.Background(), "docs", "archive/docs", expected); !errors.Is(err, ErrExists) {
		t.Errorf("Copy() collision error = %v, want %v", err, ErrExists)
	}
	if _, err := store.Copy(context.Background(), "docs", "docs/copy", expected); err == nil {
		t.Fatal("Copy() into source error = nil, want rejection")
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "copy")); !os.IsNotExist(err) {
		t.Errorf("nested copy stat error = %v, want not exist", err)
	}
	assertNoStagingDirectories(t, filepath.Join(root, "archive"))
}

func TestCopyEntryRejectsMutationStagingDirectory(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"source/" + mutationStagingPrefix + "race"}, nil)
	store, err := NewStore(root, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := store.openParent("")
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()

	err = copyEntry(context.Background(), int(directory.Fd()), "source", int(directory.Fd()), "copy", identityFor(t, store, "source"), false)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("copyEntry() staging child error = %v, want %v", err, ErrConflict)
	}
}

func TestMoveDirectoryMovesTreeWithoutOverwritingOrNestingInItself(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"docs/guide", "other", "self/child", "archive"}, []treeFile{
		{path: "docs/guide/note.md", content: "note", mode: 0o600},
		{path: "other/keep.md", content: "keep", mode: 0o600},
	})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.Move(context.Background(), "docs", "archive/docs", identityFor(t, store, "docs"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "archive/docs" {
		t.Errorf("Move() path = %q, want archive/docs", got)
	}
	if _, err := os.Stat(filepath.Join(root, "docs")); !os.IsNotExist(err) {
		t.Errorf("source stat error = %v, want not exist", err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "archive", "docs", "guide", "note.md")); err != nil || string(content) != "note" {
		t.Errorf("moved content = %q, error = %v", content, err)
	}
	if _, err := store.Move(context.Background(), "other", "archive/docs", identityFor(t, store, "other")); !errors.Is(err, ErrExists) {
		t.Errorf("Move() collision error = %v, want %v", err, ErrExists)
	}
	if content, err := os.ReadFile(filepath.Join(root, "other", "keep.md")); err != nil || string(content) != "keep" {
		t.Errorf("collision source content = %q, error = %v", content, err)
	}
	if _, err := store.Move(context.Background(), "self", "self/child/nested", identityFor(t, store, "self")); err == nil {
		t.Fatal("Move() into source error = nil, want rejection")
	}
	if _, err := os.Stat(filepath.Join(root, "self")); err != nil {
		t.Errorf("self-nesting rejection removed source: %v", err)
	}
}

func TestMoveDirectoryRejectsCaseInsensitiveAliasNesting(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{"Docs/child"}, nil)
	if _, err := os.Stat(filepath.Join(root, "docs")); err != nil {
		t.Skip("filesystem is case-sensitive")
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Move(context.Background(), "Docs", "docs/child/nested", identityFor(t, store, "Docs")); !errors.Is(err, ErrConflict) {
		t.Fatalf("Move() alias nesting error = %v, want %v", err, ErrConflict)
	}
	if _, err := os.Stat(filepath.Join(root, "Docs")); err != nil {
		t.Errorf("alias nesting rejection removed source: %v", err)
	}
	assertNoStagingDirectories(t, root)
}

func TestStageCheckedEntryRejectsReplacementAfterPriorCheck(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "original", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := identityFor(t, store, "note.md")
	parent, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if _, err := checkedChild(int(parent.Fd()), "note.md", expected, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "note.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	staged, err := stageCheckedEntry(int(parent.Fd()), "note.md", expected)
	if staged != nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("stageCheckedEntry() = %#v, %v; want nil, %v", staged, err, ErrConflict)
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "replacement" {
		t.Fatalf("replacement content = %q, error = %v", content, err)
	}
	assertNoStagingDirectories(t, root)
}

func TestRestoreStagedNeverOverwritesReplacementAndReportsExactPath(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "original", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staged, err := stageCheckedEntry(int(parent.Fd()), "note.md", identityFor(t, store, "note.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Close()
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	cause := errors.New("final action failed")
	err = staged.restore(cause)
	stagedPath := filepath.Join(staged.path, "note.md")
	if !errors.Is(err, cause) || !errors.Is(err, ErrExists) || !strings.Contains(err.Error(), stagedPath) {
		t.Fatalf("restore error = %v, want cause, conflict and exact staged path %q", err, stagedPath)
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "replacement" {
		t.Fatalf("replacement content = %q, error = %v", content, err)
	}
	if content, err := os.ReadFile(stagedPath); err != nil || string(content) != "original" {
		t.Fatalf("staged original content = %q, error = %v", content, err)
	}
}

func assertNoStagingDirectories(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), mutationStagingPrefix) {
			t.Fatalf("staging directory %q was not cleaned", entry.Name())
		}
	}
}

func TestMutationsStayInOpenedRootAfterRootPathReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "notes")
	original := filepath.Join(parent, "notes-original")
	writeTree(t, root, nil, []treeFile{
		{path: "rename.md", content: "original", mode: 0o600},
		{path: "delete.md", content: "original", mode: 0o600},
	})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	renameIdentity := identityFor(t, store, "rename.md")
	deleteIdentity := identityFor(t, store, "delete.md")
	if err := os.Rename(root, original); err != nil {
		t.Fatal(err)
	}
	writeTree(t, root, nil, []treeFile{
		{path: "rename.md", content: "replacement", mode: 0o600},
		{path: "delete.md", content: "replacement", mode: 0o600},
	})

	if _, err := store.Rename(context.Background(), "rename.md", "renamed.md", renameIdentity); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "delete.md", deleteIdentity); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "rename.md")); err != nil || string(content) != "replacement" {
		t.Errorf("replacement root rename source = %q, error = %v", content, err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "delete.md")); err != nil || string(content) != "replacement" {
		t.Errorf("replacement root delete source = %q, error = %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(original, "renamed.md")); err != nil {
		t.Errorf("opened root renamed note stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(original, "delete.md")); !os.IsNotExist(err) {
		t.Errorf("opened root deleted note stat error = %v, want not exist", err)
	}
}

func TestDeletePermanentlyRemovesFileAndNonEmptyDirectoryWithoutFollowingSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTree(t, root, []string{"folder/nested"}, []treeFile{
		{path: "note.md", content: "note", mode: 0o600},
		{path: "folder/nested/child.md", content: "child", mode: 0o600},
	})
	writeTree(t, outside, nil, []treeFile{{path: "keep.md", content: "keep", mode: 0o600}})
	makeSymlink(t, filepath.Join(outside, "keep.md"), filepath.Join(root, "folder", "outside.md"))
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(context.Background(), "note.md", identityFor(t, store, "note.md")); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "folder", identityFor(t, store, "folder")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"note.md", "folder"} {
		if _, err := os.Lstat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Errorf("deleted %q stat error = %v, want not exist", path, err)
		}
	}
	if content, err := os.ReadFile(filepath.Join(outside, "keep.md")); err != nil || string(content) != "keep" {
		t.Errorf("symlink target content = %q, error = %v", content, err)
	}
}

func TestDeleteRejectsChangedMissingAndSymlinkReplacedSource(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "old", mode: 0o600}})
	writeTree(t, outside, nil, []treeFile{{path: "outside.md", content: "outside", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := identityFor(t, store, "note.md")

	if err := os.Remove(filepath.Join(root, "note.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "note.md", expected); !errors.Is(err, ErrConflict) {
		t.Errorf("Delete() replaced source error = %v, want %v", err, ErrConflict)
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "replacement" {
		t.Errorf("replacement content = %q, error = %v", content, err)
	}

	if err := os.Remove(filepath.Join(root, "note.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.md"), filepath.Join(root, "note.md")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if err := store.Delete(context.Background(), "note.md", expected); !errors.Is(err, ErrConflict) {
		t.Errorf("Delete() symlink replacement error = %v, want %v", err, ErrConflict)
	}
	if content, err := os.ReadFile(filepath.Join(outside, "outside.md")); err != nil || string(content) != "outside" {
		t.Errorf("symlink target content = %q, error = %v", content, err)
	}
	if err := os.Remove(filepath.Join(root, "note.md")); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "note.md", expected); !errors.Is(err, ErrConflict) {
		t.Errorf("Delete() missing source error = %v, want %v", err, ErrConflict)
	}
}

func TestDeleteCanceledBeforeDestructivePhaseLeavesEntryUntouched(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "note.md", content: "keep", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.Delete(ctx, "note.md", identityFor(t, store, "note.md")); !errors.Is(err, context.Canceled) {
		t.Errorf("Delete() error = %v, want %v", err, context.Canceled)
	}
	if content, err := os.ReadFile(filepath.Join(root, "note.md")); err != nil || string(content) != "keep" {
		t.Errorf("canceled delete content = %q, error = %v", content, err)
	}
}

func identityFor(t *testing.T, store *Store, path RelPath) FileIdentity {
	t.Helper()
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range snapshot.Entries {
		if entry.Path == path {
			return entry.Identity
		}
	}
	t.Fatalf("identity for %q not found", path)
	return FileIdentity{}
}
