//go:build darwin || linux

package notes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func (s *Store) Copy(ctx context.Context, source, destination RelPath, expected FileIdentity) (RelPath, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sourceParent, sourceName, err := noteMutationPath(source)
	if err != nil {
		return "", err
	}
	destinationParent, destinationName, err := noteMutationPath(destination)
	if err != nil {
		return "", err
	}
	sourceDirectory, err := s.openParent(sourceParent)
	if err != nil {
		return "", fmt.Errorf("open copy source parent %q: %w", sourceParent, err)
	}
	defer sourceDirectory.Close()
	info, err := checkedChild(int(sourceDirectory.Fd()), sourceName, expected, false)
	if err != nil || !info.regular {
		if err == nil {
			err = errors.New("source is not a regular note")
		}
		return "", fmt.Errorf("copy %q: %w", source, err)
	}
	sourceFile, err := openChild(int(sourceDirectory.Fd()), sourceName, false)
	if err != nil {
		return "", fmt.Errorf("open copy source %q: %w", source, err)
	}
	defer sourceFile.Close()
	if opened, err := fileIdentity(sourceFile); err != nil || opened != expected {
		if err == nil {
			err = ErrConflict
		}
		return "", fmt.Errorf("copy %q: %w", source, err)
	}
	destinationDirectory, err := s.openParent(destinationParent)
	if err != nil {
		return "", fmt.Errorf("open copy target parent %q: %w", destinationParent, err)
	}
	defer destinationDirectory.Close()
	staged, err := createMutationStaging(int(destinationDirectory.Fd()), destinationName)
	if err != nil {
		return "", fmt.Errorf("stage copy target %q: %w", destination, err)
	}
	defer staged.Close()
	stagedFD := int(staged.directory.Fd())
	target, err := createFile(stagedFD, destinationName)
	if err != nil {
		return "", staged.closeAndRemove(fmt.Errorf("create staged copy %q: %w", destination, err))
	}
	discard := func(cause error) error {
		if err := unix.Unlinkat(stagedFD, destinationName, 0); err != nil {
			cause = errors.Join(cause, cleanupError(filepath.Join(staged.path, destinationName), err))
		}
		return staged.remove(cause)
	}
	if _, err := io.Copy(target, sourceFile); err != nil {
		return "", discard(errors.Join(fmt.Errorf("copy %q to %q: %w", source, destination, err), target.Close()))
	}
	if err := ctx.Err(); err != nil {
		return "", discard(errors.Join(err, target.Close()))
	}
	if err := target.Close(); err != nil {
		return "", discard(fmt.Errorf("close copied note %q: %w", destination, err))
	}
	if err := renameNoReplace(stagedFD, destinationName, int(destinationDirectory.Fd()), destinationName); err != nil {
		cause := err
		if errors.Is(err, unix.EEXIST) {
			cause = ErrExists
		}
		return "", discard(fmt.Errorf("copy %q to %q: %w", source, destination, cause))
	}
	if err := staged.remove(nil); err != nil {
		return "", err
	}
	return destination, nil
}

func (s *Store) Move(ctx context.Context, source, destination RelPath, expected FileIdentity) (RelPath, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sourceParent, sourceName, err := noteMutationPath(source)
	if err != nil {
		return "", err
	}
	destinationParent, destinationName, err := noteMutationPath(destination)
	if err != nil {
		return "", err
	}
	sourceDirectory, err := s.openParent(sourceParent)
	if err != nil {
		return "", fmt.Errorf("open move source parent %q: %w", sourceParent, err)
	}
	defer sourceDirectory.Close()
	info, err := checkedChild(int(sourceDirectory.Fd()), sourceName, expected, false)
	if err != nil || !info.regular {
		if err == nil {
			err = errors.New("source is not a regular note")
		}
		return "", fmt.Errorf("move %q: %w", source, err)
	}
	if source == destination {
		return source, nil
	}
	destinationDirectory, err := s.openParent(destinationParent)
	if err != nil {
		return "", fmt.Errorf("open move target parent %q: %w", destinationParent, err)
	}
	defer destinationDirectory.Close()
	staged, err := stageCheckedEntry(int(sourceDirectory.Fd()), sourceName, expected)
	if err != nil {
		return "", fmt.Errorf("move %q: %w", source, err)
	}
	defer staged.Close()
	if err := ctx.Err(); err != nil {
		return "", staged.restore(err)
	}
	if err := renameNoReplace(int(staged.directory.Fd()), sourceName, int(destinationDirectory.Fd()), destinationName); err != nil {
		cause := err
		switch {
		case errors.Is(err, unix.EEXIST):
			cause = ErrExists
		case errors.Is(err, unix.ENOENT):
			cause = ErrConflict
		}
		return "", staged.restore(fmt.Errorf("move %q to %q: %w", source, destination, cause))
	}
	if err := staged.remove(nil); err != nil {
		return "", err
	}
	return destination, nil
}

func noteMutationPath(value RelPath) (RelPath, string, error) {
	parent, name, err := mutationParent(value)
	if err != nil {
		return "", "", err
	}
	if err := validDirectoryName(name); err != nil {
		return "", "", err
	}
	if !strings.EqualFold(path.Ext(name), ".md") {
		return "", "", fmt.Errorf("note path %q must end in .md", value)
	}
	return parent, name, nil
}
