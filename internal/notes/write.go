//go:build darwin || linux

package notes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const stagedReplacementName = "replacement"

type WriteResult struct {
	Path     RelPath
	Revision Revision
	Created  bool
}

func (s *Store) Write(ctx context.Context, path RelPath, content []byte, expected *Revision) (WriteResult, error) {
	if err := ctx.Err(); err != nil {
		return WriteResult{}, err
	}
	parent, name, err := copyMovePath(path)
	if err != nil {
		return WriteResult{}, err
	}
	if err := validateNotePath(path, name); err != nil {
		return WriteResult{}, err
	}
	if expected != nil {
		parsed, err := ParseRevision(string(*expected))
		if err != nil {
			return WriteResult{}, err
		}
		expected = &parsed
	}
	directory, err := s.openParent(parent)
	if err != nil {
		return WriteResult{}, fmt.Errorf("open write parent %q: %w", parent, err)
	}
	defer directory.Close()
	staged, err := createMutationStaging(int(directory.Fd()), name)
	if err != nil {
		return WriteResult{}, fmt.Errorf("stage write %q: %w", path, err)
	}
	defer staged.Close()
	if err := prepareReplacement(staged, content); err != nil {
		return WriteResult{}, discardReplacement(staged, fmt.Errorf("prepare write %q: %w", path, err))
	}
	if err := ctx.Err(); err != nil {
		return WriteResult{}, discardReplacement(staged, err)
	}
	if expected == nil {
		return commitCreate(path, content, staged)
	}
	return commitUpdate(ctx, path, content, *expected, staged)
}

func prepareReplacement(staged *stagedEntry, content []byte) (err error) {
	file, err := createFile(int(staged.directory.Fd()), stagedReplacementName)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err := io.Copy(file, bytes.NewReader(content)); err != nil {
		return err
	}
	return file.Sync()
}

func commitCreate(path RelPath, content []byte, staged *stagedEntry) (WriteResult, error) {
	err := renameNoReplace(int(staged.directory.Fd()), stagedReplacementName, staged.parentFD, staged.name)
	if err != nil {
		cause := err
		if errors.Is(err, unix.EEXIST) {
			cause = ErrExists
		}
		return WriteResult{}, discardReplacement(staged, fmt.Errorf("create note %q: %w", path, cause))
	}
	if err := staged.remove(nil); err != nil {
		return WriteResult{}, errors.Join(ErrRecoveryRequired, fmt.Errorf("finish creating note %q: %w", path, err))
	}
	return WriteResult{Path: path, Revision: revisionBytes(content), Created: true}, nil
}

func commitUpdate(ctx context.Context, path RelPath, content []byte, expected Revision, staged *stagedEntry) (WriteResult, error) {
	current, err := lstatChild(staged.parentFD, staged.name)
	if errors.Is(err, unix.ENOENT) {
		return WriteResult{}, discardReplacement(staged, &RevisionConflictError{Expected: expected, Actual: ""})
	}
	if err != nil {
		return WriteResult{}, discardReplacement(staged, fmt.Errorf("stat note %q: %w", path, err))
	}
	if !current.regular || current.symlink {
		return WriteResult{}, discardReplacement(staged, fmt.Errorf("%w: %q", ErrNotRegularFile, path))
	}
	if err := staged.stage(current.identity); err != nil {
		return WriteResult{}, discardReplacement(staged, fmt.Errorf("stage note %q: %w", path, err))
	}
	if err := ctx.Err(); err != nil {
		return WriteResult{}, restoreAfterDiscard(staged, err)
	}
	actual, err := stagedRevision(staged)
	if err != nil {
		return WriteResult{}, restoreAfterDiscard(staged, fmt.Errorf("revision note %q: %w", path, err))
	}
	if actual != expected {
		return WriteResult{}, restoreAfterDiscard(staged, &RevisionConflictError{Expected: expected, Actual: actual})
	}
	if err := renameNoReplace(int(staged.directory.Fd()), stagedReplacementName, staged.parentFD, staged.name); err != nil {
		cause := err
		if errors.Is(err, unix.EEXIST) {
			cause = ErrExists
		}
		return WriteResult{}, restoreAfterDiscard(staged, fmt.Errorf("replace note %q: %w", path, cause))
	}
	if err := unix.Unlinkat(int(staged.directory.Fd()), staged.name, 0); err != nil {
		return WriteResult{}, errors.Join(ErrRecoveryRequired, fmt.Errorf("remove old note staged at %q: %w", filepath.Join(staged.path, staged.name), err))
	}
	if err := staged.remove(nil); err != nil {
		return WriteResult{}, errors.Join(ErrRecoveryRequired, fmt.Errorf("finish replacing note %q: %w", path, err))
	}
	return WriteResult{Path: path, Revision: revisionBytes(content), Created: false}, nil
}

func stagedRevision(staged *stagedEntry) (revision Revision, err error) {
	file, err := openChild(int(staged.directory.Fd()), staged.name, false)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	return revisionFile(file)
}

func restoreAfterDiscard(staged *stagedEntry, cause error) error {
	if err := removeReplacement(staged); err != nil {
		cause = errors.Join(cause, err)
	}
	return staged.restore(cause)
}

func discardReplacement(staged *stagedEntry, cause error) error {
	if err := removeReplacement(staged); err != nil {
		cause = errors.Join(cause, err)
	}
	return staged.remove(cause)
}

func removeReplacement(staged *stagedEntry) error {
	err := unix.Unlinkat(int(staged.directory.Fd()), stagedReplacementName, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	return err
}
