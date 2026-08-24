//go:build darwin || linux

package notes

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const mutationStagingPrefix = ".jotmd-stage-"

type stagedEntry struct {
	parentFD  int
	directory *os.File
	dirName   string
	path      string
	name      string
}

func stageCheckedEntry(parentFD int, name string, expected FileIdentity) (*stagedEntry, error) {
	staged, err := createMutationStaging(parentFD, name)
	if err != nil {
		return nil, err
	}
	if err := staged.stage(expected); err != nil {
		return nil, staged.closeAndRemove(err)
	}
	return staged, nil
}

func (staged *stagedEntry) stage(expected FileIdentity) error {
	if err := renameNoReplace(staged.parentFD, staged.name, int(staged.directory.Fd()), staged.name); err != nil {
		cause := err
		if errors.Is(err, unix.ENOENT) {
			cause = ErrConflict
		}
		return fmt.Errorf("stage entry: %w", cause)
	}
	if _, err := checkedChild(int(staged.directory.Fd()), staged.name, expected, false); err != nil {
		cause := fmt.Errorf("verify staged entry: %w", err)
		return staged.restoreEntry(cause)
	}
	return nil
}

func createMutationStaging(parentFD int, name string) (*stagedEntry, error) {
	for {
		dirName, err := randomStagingName()
		if err != nil {
			return nil, fmt.Errorf("prepare staging name: %w", err)
		}
		if err := unix.Mkdirat(parentFD, dirName, 0o700); errors.Is(err, unix.EEXIST) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("create staging directory: %w", err)
		}
		directory, err := openChild(parentFD, dirName, true)
		if err != nil {
			cleanupErr := unix.Unlinkat(parentFD, dirName, unix.AT_REMOVEDIR)
			return nil, errors.Join(fmt.Errorf("open staging directory: %w", err), cleanupError(dirName, cleanupErr))
		}
		path, err := descriptorPath(int(directory.Fd()))
		if err != nil {
			_ = directory.Close()
			cleanupErr := unix.Unlinkat(parentFD, dirName, unix.AT_REMOVEDIR)
			return nil, errors.Join(fmt.Errorf("resolve staging directory: %w", err), cleanupError(dirName, cleanupErr))
		}
		return &stagedEntry{parentFD: parentFD, directory: directory, dirName: dirName, path: path, name: name}, nil
	}
}

func (staged *stagedEntry) restore(cause error) error {
	cause = staged.restoreEntry(cause)
	if errors.Is(cause, ErrRecoveryRequired) {
		return cause
	}
	return staged.remove(cause)
}

func (staged *stagedEntry) restoreEntry(cause error) error {
	err := renameNoReplace(int(staged.directory.Fd()), staged.name, staged.parentFD, staged.name)
	if err != nil {
		if errors.Is(err, unix.EEXIST) {
			err = ErrExists
		}
		return errors.Join(cause, ErrRecoveryRequired, fmt.Errorf("restore failed; source remains staged at %q: %w", filepath.Join(staged.path, staged.name), err))
	}
	return cause
}

func (staged *stagedEntry) remove(cause error) error {
	if err := unix.Unlinkat(staged.parentFD, staged.dirName, unix.AT_REMOVEDIR); err != nil {
		return errors.Join(cause, fmt.Errorf("remove empty staging directory %q: %w", staged.path, err))
	}
	return cause
}

func (staged *stagedEntry) closeAndRemove(cause error) error {
	err := staged.remove(cause)
	return errors.Join(err, staged.Close())
}

func (staged *stagedEntry) Close() error {
	if staged.directory == nil {
		return nil
	}
	err := staged.directory.Close()
	staged.directory = nil
	return err
}

func randomStagingName() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return mutationStagingPrefix + hex.EncodeToString(random[:]), nil
}

func cleanupError(path string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("remove empty staging directory %q: %w", path, err)
}
