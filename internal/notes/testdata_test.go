package notes

import (
	"os"
	"path/filepath"
	"testing"
)

type treeFile struct {
	path    string
	content string
	mode    os.FileMode
}

func writeTree(t *testing.T, root string, dirs []string, files []treeFile) {
	t.Helper()

	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range files {
		path := filepath.Join(root, file.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file.content), file.mode); err != nil {
			t.Fatal(err)
		}
	}
}

func makeSymlink(t *testing.T, target, path string) {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
}

func entryPaths(entries []Entry) []RelPath {
	paths := make([]RelPath, len(entries))
	for i, entry := range entries {
		paths[i] = entry.Path
	}
	return paths
}
