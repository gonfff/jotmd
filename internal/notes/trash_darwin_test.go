//go:build darwin

package notes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFinderTrashCommandPassesPathAsOpaqueArgument(t *testing.T) {
	path := `/tmp/note " & do shell script "touch /tmp/pwned" & ".md`
	command := finderTrashCommand(context.Background(), path)
	want := []string{
		"/usr/bin/osascript",
		"-e",
		"on run argv\ntell application \"Finder\" to delete POSIX file (item 1 of argv)\nend run",
		"--",
		path,
	}
	if command.Path != "/usr/bin/osascript" || !reflect.DeepEqual(command.Args, want) {
		t.Errorf("finderTrashCommand() path/args = %q, %#v, want %q, %#v", command.Path, command.Args, "/usr/bin/osascript", want)
	}
}

func TestTrashRejectsChangedSourceBeforeInvokingFinder(t *testing.T) {
	root := t.TempDir()
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
		t.Errorf("Trash() error = %v, want %v", err, ErrConflict)
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != "replacement" {
		t.Errorf("replacement content = %q, error = %v", content, err)
	}
}

func TestDescriptorPathFollowsMovedOpenDirectory(t *testing.T) {
	parent := t.TempDir()
	original := filepath.Join(parent, "original")
	moved := filepath.Join(parent, "moved")
	if err := os.Mkdir(original, 0o755); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(original)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}

	got, err := descriptorPath(int(directory.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(moved)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("descriptorPath() = %q, want moved path %q", got, want)
	}
}

func TestTrashStagesCheckedSourceBeforeFinderAndDoesNotTrashReplacement(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(root, "note.md")
	if err := os.WriteFile(originalPath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := identityFor(t, store, "note.md")
	finderFailure := errors.New("finder failed")
	previous := runFinderTrash
	var stagedPath string
	runFinderTrash = func(_ context.Context, path string) error {
		stagedPath = path
		content, err := os.ReadFile(path)
		if err != nil || string(content) != "original" {
			t.Errorf("staged content = %q, error = %v", content, err)
		}
		if err := os.WriteFile(originalPath, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
		return finderFailure
	}
	t.Cleanup(func() { runFinderTrash = previous })

	trashErr := store.Trash(context.Background(), "note.md", expected)
	if !errors.Is(trashErr, finderFailure) {
		t.Errorf("Trash() error = %v, want Finder failure", trashErr)
	}
	if content, readErr := os.ReadFile(originalPath); readErr != nil || string(content) != "replacement" {
		t.Errorf("replacement content = %q, error = %v", content, readErr)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if stagedPath == "" || filepath.Base(stagedPath) != "note.md" || filepath.Dir(filepath.Dir(stagedPath)) != canonicalRoot || !strings.HasPrefix(filepath.Base(filepath.Dir(stagedPath)), mutationStagingPrefix) {
		t.Fatalf("staged path = %q, want original basename under unpredictable staging directory in %q", stagedPath, root)
	}
	if content, readErr := os.ReadFile(stagedPath); readErr != nil || string(content) != "original" {
		t.Errorf("preserved staged content = %q, error = %v", content, readErr)
	}
	if !strings.Contains(trashErr.Error(), stagedPath) {
		t.Errorf("Trash() error = %q, want resulting staging path %q", trashErr, stagedPath)
	}
}

func TestTrashRemovesEmptyStagingDirectoryAfterFinderSuccess(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	previous := runFinderTrash
	var stagingPath string
	runFinderTrash = func(_ context.Context, path string) error {
		stagingPath = filepath.Dir(path)
		return os.Remove(path)
	}
	t.Cleanup(func() { runFinderTrash = previous })

	if err := store.Trash(context.Background(), "note.md", identityFor(t, store, "note.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("staging directory stat error = %v, want not exist", err)
	}
}

func TestFinishStagedTrashRejectsWrongIdentityWithoutInvokingFinder(t *testing.T) {
	root := t.TempDir()
	stagingName := ".jotmd-trash-stage-test"
	stagingPath := filepath.Join(root, stagingName)
	stagedPath := filepath.Join(stagingPath, "note.md")
	originalPath := filepath.Join(root, "note.md")
	if err := os.Mkdir(stagingPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("unexpected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(originalPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := os.Open(stagingPath)
	if err != nil {
		t.Fatal(err)
	}
	defer staging.Close()
	previous := runFinderTrash
	finderCalled := false
	runFinderTrash = func(context.Context, string) error {
		finderCalled = true
		return nil
	}
	t.Cleanup(func() { runFinderTrash = previous })

	staged := &stagedEntry{parentFD: int(parent.Fd()), directory: staging, dirName: stagingName, path: stagingPath, name: "note.md"}
	err = finishStagedTrash(context.Background(), "note.md", FileIdentity{}, staged)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("finishStagedTrash() error = %v, want %v", err, ErrConflict)
	}
	if finderCalled {
		t.Error("Finder invoked for staged entry with wrong identity")
	}
	if content, readErr := os.ReadFile(originalPath); readErr != nil || string(content) != "replacement" {
		t.Errorf("replacement content = %q, error = %v", content, readErr)
	}
	if content, readErr := os.ReadFile(stagedPath); readErr != nil || string(content) != "unexpected" {
		t.Errorf("staged content = %q, error = %v", content, readErr)
	}
	if !strings.Contains(err.Error(), stagedPath) {
		t.Errorf("finishStagedTrash() error = %q, want staged path %q", err, stagedPath)
	}
}
