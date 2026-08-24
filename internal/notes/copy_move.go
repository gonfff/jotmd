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
	sourceParent, sourceName, err := copyMovePath(source)
	if err != nil {
		return "", err
	}
	destinationParent, destinationName, err := copyMovePath(destination)
	if err != nil {
		return "", err
	}
	sourceDirectory, err := s.openParent(sourceParent)
	if err != nil {
		return "", fmt.Errorf("open copy source parent %q: %w", sourceParent, err)
	}
	defer sourceDirectory.Close()
	info, err := checkedChild(int(sourceDirectory.Fd()), sourceName, expected, false)
	if err != nil || !info.regular && !info.directory {
		if err == nil {
			err = errors.New("source is not a note or directory")
		}
		return "", fmt.Errorf("copy %q: %w", source, err)
	}
	if info.regular {
		if err := validateNotePath(source, sourceName); err != nil {
			return "", err
		}
		if err := validateNotePath(destination, destinationName); err != nil {
			return "", err
		}
	}
	destinationDirectory, err := s.openParent(destinationParent)
	if err != nil {
		return "", fmt.Errorf("open copy target parent %q: %w", destinationParent, err)
	}
	defer destinationDirectory.Close()
	if info.directory {
		inside, err := s.directoryWithinSource(int(destinationDirectory.Fd()), expected)
		if err != nil {
			return "", fmt.Errorf("check copy target parent %q: %w", destinationParent, err)
		}
		if inside {
			return "", fmt.Errorf("copy %q into itself: %w", source, ErrConflict)
		}
	}
	staged, err := createMutationStaging(int(destinationDirectory.Fd()), destinationName)
	if err != nil {
		return "", fmt.Errorf("stage copy target %q: %w", destination, err)
	}
	defer staged.Close()
	stagedFD := int(staged.directory.Fd())
	discard := func(cause error) error {
		if copied, err := lstatChild(stagedFD, destinationName); err == nil {
			cause = errors.Join(cause, deleteEntry(stagedFD, destinationName, copied.identity, true))
		} else if !errors.Is(err, unix.ENOENT) {
			cause = errors.Join(cause, cleanupError(filepath.Join(staged.path, destinationName), err))
		}
		return staged.remove(cause)
	}
	if err := copyEntry(ctx, int(sourceDirectory.Fd()), sourceName, stagedFD, destinationName, expected, false); err != nil {
		return "", discard(fmt.Errorf("copy %q to %q: %w", source, destination, err))
	}
	if info.directory {
		inside, err := s.directoryWithinSource(int(destinationDirectory.Fd()), expected)
		if err != nil {
			return "", discard(fmt.Errorf("recheck copy target parent %q: %w", destinationParent, err))
		}
		if inside {
			return "", discard(fmt.Errorf("copy %q into itself: %w", source, ErrConflict))
		}
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

func copyEntry(ctx context.Context, sourceFD int, sourceName string, destinationFD int, destinationName string, expected FileIdentity, allowSymlink bool) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(sourceName, mutationStagingPrefix) {
		return ErrConflict
	}
	info, err := checkedChild(sourceFD, sourceName, expected, allowSymlink)
	if err != nil {
		return err
	}
	if info.symlink {
		target, err := readlinkAt(sourceFD, sourceName)
		if err != nil {
			return err
		}
		if _, err := checkedChild(sourceFD, sourceName, expected, true); err != nil {
			return err
		}
		return unix.Symlinkat(target, destinationFD, destinationName)
	}
	if !info.regular && !info.directory {
		return errors.New("unsupported filesystem entry")
	}

	source, err := openChild(sourceFD, sourceName, info.directory)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, source.Close()) }()
	if opened, err := fileIdentity(source); err != nil || opened != expected {
		if err == nil {
			err = ErrConflict
		}
		return err
	}
	if info.regular {
		target, err := createFile(destinationFD, destinationName)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, target.Close()) }()
		if _, err := io.Copy(target, source); err != nil {
			return err
		}
		return ctx.Err()
	}

	if err := unix.Mkdirat(destinationFD, destinationName, 0o777); err != nil {
		return err
	}
	target, err := openChild(destinationFD, destinationName, true)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, target.Close()) }()
	entries, err := readDirectory(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child, err := lstatChild(int(source.Fd()), entry.Name())
		if err != nil {
			return err
		}
		if err := copyEntry(ctx, int(source.Fd()), entry.Name(), int(target.Fd()), entry.Name(), child.identity, true); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func readlinkAt(directoryFD int, name string) (string, error) {
	for size := 128; ; size *= 2 {
		buffer := make([]byte, size)
		length, err := unix.Readlinkat(directoryFD, name, buffer)
		if err != nil {
			return "", err
		}
		if length < len(buffer) {
			return string(buffer[:length]), nil
		}
	}
}

func (s *Store) Move(ctx context.Context, source, destination RelPath, expected FileIdentity) (RelPath, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sourceParent, sourceName, err := copyMovePath(source)
	if err != nil {
		return "", err
	}
	destinationParent, destinationName, err := copyMovePath(destination)
	if err != nil {
		return "", err
	}
	sourceDirectory, err := s.openParent(sourceParent)
	if err != nil {
		return "", fmt.Errorf("open move source parent %q: %w", sourceParent, err)
	}
	defer sourceDirectory.Close()
	info, err := checkedChild(int(sourceDirectory.Fd()), sourceName, expected, false)
	if err != nil || !info.regular && !info.directory {
		if err == nil {
			err = errors.New("source is not a note or directory")
		}
		return "", fmt.Errorf("move %q: %w", source, err)
	}
	if info.regular {
		if err := validateNotePath(source, sourceName); err != nil {
			return "", err
		}
		if err := validateNotePath(destination, destinationName); err != nil {
			return "", err
		}
	}
	if source == destination {
		return source, nil
	}
	destinationDirectory, err := s.openParent(destinationParent)
	if err != nil {
		return "", fmt.Errorf("open move target parent %q: %w", destinationParent, err)
	}
	defer destinationDirectory.Close()
	if info.directory {
		inside, err := s.directoryWithinSource(int(destinationDirectory.Fd()), expected)
		if err != nil {
			return "", fmt.Errorf("check move target parent %q: %w", destinationParent, err)
		}
		if inside {
			return "", fmt.Errorf("move %q into itself: %w", source, ErrConflict)
		}
	}
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

func copyMovePath(value RelPath) (RelPath, string, error) {
	parent, name, err := mutationParent(value)
	if err != nil {
		return "", "", err
	}
	if err := validDirectoryName(name); err != nil {
		return "", "", err
	}
	return parent, name, nil
}

func validateNotePath(value RelPath, name string) error {
	if !strings.EqualFold(path.Ext(name), ".md") {
		return fmt.Errorf("note path %q must end in .md", value)
	}
	return nil
}

func (s *Store) directoryWithinSource(directoryFD int, source FileIdentity) (inside bool, err error) {
	var rootStat unix.Stat_t
	if err := unix.Fstat(s.rootFD, &rootStat); err != nil {
		return false, err
	}
	root := identityFromStat(&rootStat)
	current, err := duplicateFile(directoryFD, ".")
	if err != nil {
		return false, err
	}
	defer func() { err = errors.Join(err, current.Close()) }()
	for {
		identity, err := fileIdentity(current)
		if err != nil {
			return false, err
		}
		if identity == source {
			return true, nil
		}
		if identity == root {
			return false, nil
		}
		parent, err := openChild(int(current.Fd()), "..", true)
		if err != nil {
			return false, err
		}
		parentIdentity, err := fileIdentity(parent)
		if err != nil {
			_ = parent.Close()
			return false, err
		}
		if parentIdentity == identity {
			_ = parent.Close()
			return false, ErrConflict
		}
		previous := current
		current = parent
		if err := previous.Close(); err != nil {
			return false, err
		}
	}
}
