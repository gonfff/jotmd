package notes

import (
	"fmt"
	"os"
)

type RelPath string

func (s *Store) AbsolutePath(path RelPath) (string, error) {
	file, err := openRelative(s.rootFD, path, false)
	if err != nil {
		return "", fmt.Errorf("open note %q: %w", path, err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat note %q: %w", path, err)
	}
	if !opened.Mode().IsRegular() {
		return "", fmt.Errorf("note %q is not a regular file", path)
	}
	absolute, err := descriptorPath(int(file.Fd()))
	if err != nil {
		return "", fmt.Errorf("resolve note %q: %w", path, err)
	}
	current, err := os.Stat(absolute)
	if err != nil || !os.SameFile(opened, current) {
		return "", fmt.Errorf("note %q changed while resolving its path", path)
	}
	return absolute, nil
}
