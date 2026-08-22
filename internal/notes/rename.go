package notes

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

func (s *Store) Rename(ctx context.Context, path RelPath, newName string, expected FileIdentity) (RelPath, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validDirectoryName(newName); err != nil {
		return "", err
	}
	parent, oldName, err := mutationParent(path)
	if err != nil {
		return "", err
	}
	directory, err := s.openParent(parent)
	if err != nil {
		return "", fmt.Errorf("open rename parent %q: %w", parent, err)
	}
	defer directory.Close()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	directoryFD := int(directory.Fd())
	if oldName == newName {
		if _, err := checkedChild(directoryFD, oldName, expected, false); err != nil {
			return "", fmt.Errorf("rename %q: %w", path, err)
		}
		return path, nil
	}
	staged, err := stageCheckedEntry(directoryFD, oldName, expected)
	if err != nil {
		return "", fmt.Errorf("rename %q: %w", path, err)
	}
	defer staged.Close()
	if err := ctx.Err(); err != nil {
		return "", staged.restore(err)
	}
	if err := renameNoReplace(int(staged.directory.Fd()), oldName, directoryFD, newName); err != nil {
		cause := err
		switch {
		case errors.Is(err, unix.EEXIST):
			cause = ErrExists
		case errors.Is(err, unix.ENOENT):
			cause = ErrConflict
		}
		return "", staged.restore(fmt.Errorf("rename %q to %q: %w", path, newName, cause))
	}
	if err := staged.remove(nil); err != nil {
		return "", err
	}
	return joinRelPath(parent, newName), nil
}
