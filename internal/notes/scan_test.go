package notes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestScanEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root, []string{".git", ".obsidian"}, false)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Root != root {
		t.Errorf("Snapshot.Root = %q, want %q", snapshot.Root, root)
	}
	if len(snapshot.Entries) != 0 {
		t.Errorf("Snapshot.Entries = %#v, want empty", snapshot.Entries)
	}
}

func TestScanIncludesOnlyNavigableDirectoriesAndMarkdown(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root,
		[]string{"docs", ".git", ".obsidian", ".private", "ignored"},
		[]treeFile{
			{path: "docs/Nested.MD", content: "nested", mode: 0o600},
			{path: "visible.md", content: "visible", mode: 0o600},
			{path: "attachment.pdf", content: "pdf", mode: 0o600},
			{path: ".private/hidden.md", content: "hidden", mode: 0o600},
			{path: ".git/config.md", content: "git", mode: 0o600},
			{path: ".obsidian/config.md", content: "obsidian", mode: 0o600},
			{path: "ignored/skip.md", content: "skip", mode: 0o600},
			{path: "unreadable.md", content: "private", mode: 0o000},
		},
	)
	t.Cleanup(func() {
		if err := os.Chmod(filepath.Join(root, "unreadable.md"), 0o600); err != nil {
			t.Error(err)
		}
	})
	makeSymlink(t, "visible.md", filepath.Join(root, "linked.md"))
	makeSymlink(t, "docs", filepath.Join(root, "linked-dir"))

	store, err := NewStore(root, []string{".git", ".obsidian", "ignored"}, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := []RelPath{"docs", "docs/Nested.MD", "unreadable.md", "visible.md"}
	if got := entryPaths(snapshot.Entries); !reflect.DeepEqual(got, want) {
		t.Errorf("Scan() paths = %#v, want %#v", got, want)
	}
	for _, entry := range snapshot.Entries {
		if entry.Identity == (FileIdentity{}) {
			t.Errorf("Scan() identity for %q is empty", entry.Path)
		}
	}
}

func TestScanIgnoresInternalAndExternalSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTree(t, root, []string{"notes"}, []treeFile{{path: "notes/inside.md", content: "inside", mode: 0o600}})
	writeTree(t, outside, nil, []treeFile{{path: "outside.md", content: "outside", mode: 0o600}})
	makeSymlink(t, "notes/inside.md", filepath.Join(root, "inside-link.md"))
	makeSymlink(t, "notes", filepath.Join(root, "inside-link"))
	makeSymlink(t, filepath.Join(outside, "outside.md"), filepath.Join(root, "outside-link.md"))
	makeSymlink(t, outside, filepath.Join(root, "outside-link"))

	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []RelPath{"notes", "notes/inside.md"}
	if got := entryPaths(snapshot.Entries); !reflect.DeepEqual(got, want) {
		t.Errorf("Scan() paths = %#v, want %#v", got, want)
	}
}

func TestOpenScannedDirectoryRejectsReplacedOrSymlinkedChild(t *testing.T) {
	for _, test := range []struct {
		name    string
		replace func(*testing.T, string)
		want    error
	}{
		{
			name: "replacement",
			replace: func(t *testing.T, child string) {
				t.Helper()
				if err := os.Rename(child, child+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(child, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			want: ErrConflict,
		},
		{
			name: "symlink",
			replace: func(t *testing.T, child string) {
				t.Helper()
				if err := os.Remove(child); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), child); err != nil {
					t.Skipf("symlinks are unavailable: %v", err)
				}
			},
			want: nil,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			child := filepath.Join(root, "child")
			if err := os.Mkdir(child, 0o755); err != nil {
				t.Fatal(err)
			}
			parent, err := os.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			info, err := lstatChild(int(parent.Fd()), "child")
			if err != nil {
				t.Fatal(err)
			}
			test.replace(t, child)

			directory, err := openScannedDirectory(int(parent.Fd()), "child", info.identity)
			if directory != nil {
				directory.Close()
				t.Fatal("openScannedDirectory() opened replaced child")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("openScannedDirectory() error = %v, want %v", err, test.want)
			}
			if test.want == nil && err == nil {
				t.Fatalf("openScannedDirectory() error = %v, want symlink rejection", err)
			}
		})
	}
}

func TestScanStaysInOpenedRootAfterPathReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "notes")
	backup := filepath.Join(parent, "notes-original")
	outside := t.TempDir()
	writeTree(t, root, nil, []treeFile{{path: "inside.md", content: "inside", mode: 0o600}})
	writeTree(t, outside, nil, []treeFile{{path: "outside.md", content: "outside", mode: 0o600}})
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, backup); err != nil {
		t.Fatal(err)
	}
	makeSymlink(t, outside, root)

	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []RelPath{"inside.md"}
	if got := entryPaths(snapshot.Entries); !reflect.DeepEqual(got, want) {
		t.Errorf("Scan() paths = %#v, want %#v", got, want)
	}
}

func TestScanShowsHiddenMarkdownWhenEnabled(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, nil, []treeFile{
		{path: ".private.md", content: "hidden", mode: 0o600},
		{path: ".obsidian/config.md", content: "ignored", mode: 0o600},
	})

	store, err := NewStore(root, []string{".obsidian"}, true)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := []RelPath{".private.md"}
	if got := entryPaths(snapshot.Entries); !reflect.DeepEqual(got, want) {
		t.Errorf("Scan() paths = %#v, want %#v", got, want)
	}
}

func TestScanAlwaysHidesPrivateMutationStaging(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, []string{".jotmd-stage-test"}, []treeFile{
		{path: ".private.md", content: "visible hidden note", mode: 0o600},
		{path: ".jotmd-stage-test/original.md", content: "private staging", mode: 0o600},
	})
	store, err := NewStore(root, nil, true)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := entryPaths(snapshot.Entries), []RelPath{".private.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Scan() paths = %#v, want private staging excluded %#v", got, want)
	}
}

func TestScanOrdersDirectoriesBeforeFilesByFoldedName(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root,
		[]string{"zebra", "Álbum", "alpha"},
		[]treeFile{
			{path: "b.md", content: "b", mode: 0o600},
			{path: "A.md", content: "upper", mode: 0o600},
		},
	)
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}

	for range 20 {
		snapshot, err := store.Scan(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		want := []RelPath{"alpha", "zebra", "Álbum", "A.md", "b.md"}
		if got := entryPaths(snapshot.Entries); !reflect.DeepEqual(got, want) {
			t.Errorf("Scan() paths = %#v, want %#v", got, want)
		}
	}
}

func TestEntryLessUsesOriginalNameTieBreaker(t *testing.T) {
	left := scannedChild{entry: fakeDirEntry{name: "A.md"}}
	right := scannedChild{entry: fakeDirEntry{name: "a.md"}}
	if !entryLess(left, right) {
		t.Errorf("entryLess(%q, %q) = false, want true", left.entry.Name(), right.entry.Name())
	}
}

func TestEntryLessUsesUnicodeCaseFoldTieBreaker(t *testing.T) {
	if got, want := caseFoldKey("Σ.md"), caseFoldKey("ς.md"); got != want {
		t.Fatalf("caseFoldKey(Σ.md) = %q, want %q", got, want)
	}
	left := scannedChild{entry: fakeDirEntry{name: "Σ.md"}}
	right := scannedChild{entry: fakeDirEntry{name: "ς.md"}}
	if !entryLess(left, right) {
		t.Errorf("entryLess(%q, %q) = false, want true", left.entry.Name(), right.entry.Name())
	}
}

func TestScanHonorsCanceledContext(t *testing.T) {
	store, err := NewStore(t.TempDir(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := store.Scan(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Scan() error = %v, want %v", err, context.Canceled)
	}
}

type fakeDirEntry struct {
	name string
}

func (entry fakeDirEntry) Name() string         { return entry.name }
func (fakeDirEntry) IsDir() bool                { return false }
func (fakeDirEntry) Type() os.FileMode          { return 0 }
func (fakeDirEntry) Info() (os.FileInfo, error) { return nil, errors.New("not used") }
