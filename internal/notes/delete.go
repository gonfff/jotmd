//go:build darwin || linux

package notes

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func (s *Store) Delete(ctx context.Context, path RelPath, expected FileIdentity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	parent, name, err := mutationParent(path)
	if err != nil {
		return err
	}
	directory, err := s.openParent(parent)
	if err != nil {
		return fmt.Errorf("open delete parent %q: %w", parent, err)
	}
	defer directory.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	staged, err := stageCheckedEntry(int(directory.Fd()), name, expected)
	if err != nil {
		return fmt.Errorf("delete %q: %w", path, err)
	}
	defer staged.Close()
	if err := ctx.Err(); err != nil {
		return staged.restore(err)
	}
	if err := deleteEntry(int(staged.directory.Fd()), name, expected, false); err != nil {
		return staged.restore(fmt.Errorf("delete %q: %w", path, err))
	}
	if err := staged.remove(nil); err != nil {
		return fmt.Errorf("delete %q: %w", path, err)
	}
	return nil
}

func (s *Store) DeleteRevision(ctx context.Context, path RelPath, expected Revision) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	parsed, err := ParseRevision(string(expected))
	if err != nil {
		return err
	}
	parent, name, err := mutationParent(path)
	if err != nil {
		return err
	}
	if err := validateNotePath(path, name); err != nil {
		return err
	}
	directory, err := s.openParent(parent)
	if err != nil {
		return fmt.Errorf("open delete parent %q: %w", parent, err)
	}
	defer directory.Close()
	current, err := lstatChild(int(directory.Fd()), name)
	if errors.Is(err, unix.ENOENT) {
		return &RevisionConflictError{Expected: parsed, Actual: ""}
	}
	if err != nil {
		return fmt.Errorf("stat note %q: %w", path, err)
	}
	if !current.regular || current.symlink {
		return fmt.Errorf("%w: %q", ErrNotRegularFile, path)
	}
	staged, err := stageCheckedEntry(int(directory.Fd()), name, current.identity)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return &RevisionConflictError{Expected: parsed, Actual: ""}
		}
		return fmt.Errorf("delete %q: %w", path, err)
	}
	defer staged.Close()
	if err := ctx.Err(); err != nil {
		return staged.restore(err)
	}
	actual, err := stagedRevision(staged)
	if err != nil {
		return staged.restore(fmt.Errorf("revision note %q: %w", path, err))
	}
	if actual != parsed {
		return staged.restore(&RevisionConflictError{Expected: parsed, Actual: actual})
	}
	if err := deleteEntry(int(staged.directory.Fd()), name, current.identity, false); err != nil {
		return staged.restore(fmt.Errorf("delete %q: %w", path, err))
	}
	if err := staged.remove(nil); err != nil {
		return errors.Join(ErrRecoveryRequired, fmt.Errorf("finish deleting note %q: %w", path, err))
	}
	return nil
}

func deleteEntry(parentFD int, name string, expected FileIdentity, allowSymlink bool) error {
	info, err := checkedChild(parentFD, name, expected, allowSymlink)
	if err != nil {
		return err
	}
	if !info.directory {
		if err := unix.Unlinkat(parentFD, name, 0); errors.Is(err, unix.ENOENT) {
			return ErrConflict
		} else {
			return err
		}
	}

	directory, err := openChild(parentFD, name, true)
	if err != nil {
		return err
	}
	defer directory.Close()
	openedIdentity, err := fileIdentity(directory)
	if err != nil {
		return err
	}
	if openedIdentity != expected {
		return ErrConflict
	}
	entries, err := readDirectory(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child, err := lstatChild(int(directory.Fd()), entry.Name())
		if err != nil {
			return err
		}
		if err := deleteEntry(int(directory.Fd()), entry.Name(), child.identity, true); err != nil {
			return err
		}
	}
	if _, err := checkedChild(parentFD, name, expected, false); err != nil {
		return err
	}
	if err := unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR); errors.Is(err, unix.ENOENT) {
		return ErrConflict
	} else {
		return err
	}
}

func readDirectory(directory *os.File) ([]os.DirEntry, error) {
	duplicate, err := duplicateFile(int(directory.Fd()), directory.Name())
	if err != nil {
		return nil, err
	}
	defer duplicate.Close()
	return duplicate.ReadDir(-1)
}
