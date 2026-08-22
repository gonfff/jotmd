//go:build linux

package notes

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

func (s *Store) Trash(ctx context.Context, path RelPath, expected FileIdentity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	parent, name, err := mutationParent(path)
	if err != nil {
		return err
	}
	sourceDirectory, err := s.openParent(parent)
	if err != nil {
		return fmt.Errorf("open trash parent %q: %w", parent, err)
	}
	defer sourceDirectory.Close()
	sourceFD := int(sourceDirectory.Fd())
	absoluteParent, err := descriptorPath(sourceFD)
	if err != nil {
		return fmt.Errorf("resolve trash source %q: %w", path, err)
	}

	trashRoot, err := homeTrash()
	if err != nil {
		return err
	}
	filesPath := filepath.Join(trashRoot, "files")
	infoPath := filepath.Join(trashRoot, "info")
	if err := os.MkdirAll(filesPath, 0o700); err != nil {
		return fmt.Errorf("create trash files directory: %w", err)
	}
	if err := os.MkdirAll(infoPath, 0o700); err != nil {
		return fmt.Errorf("create trash info directory: %w", err)
	}
	filesDirectory, err := openDirectory(filesPath)
	if err != nil {
		return fmt.Errorf("open trash files directory: %w", err)
	}
	defer filesDirectory.Close()
	infoDirectory, err := openDirectory(infoPath)
	if err != nil {
		return fmt.Errorf("open trash info directory: %w", err)
	}
	defer infoDirectory.Close()
	trashIdentity, err := fileIdentity(filesDirectory)
	if err != nil {
		return err
	}
	staged, err := stageCheckedEntry(sourceFD, name, expected)
	if err != nil {
		return fmt.Errorf("trash %q: %w", path, err)
	}
	defer staged.Close()
	if expected.Device != trashIdentity.Device {
		return staged.restore(ErrTrashCrossFilesystem)
	}
	info := trashInfo(filepath.Join(absoluteParent, name), time.Now())

	for suffix := 0; ; suffix++ {
		if err := ctx.Err(); err != nil {
			return staged.restore(err)
		}
		candidate := name
		if suffix > 0 {
			candidate += "." + strconv.Itoa(suffix)
		}
		infoName := candidate + ".trashinfo"
		infoFile, err := createTrashInfo(int(infoDirectory.Fd()), infoName)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return staged.restore(fmt.Errorf("create trash metadata: %w", err))
		}
		cleanupInfo := true
		if _, err = infoFile.WriteString(info); err == nil {
			err = infoFile.Close()
		} else {
			_ = infoFile.Close()
		}
		if err != nil {
			writeErr := fmt.Errorf("write trash metadata: %w", err)
			return staged.restore(removeTrashInfo(int(infoDirectory.Fd()), infoName, writeErr))
		}
		if err = ctx.Err(); err == nil {
			_, err = checkedChild(int(staged.directory.Fd()), name, expected, false)
		}
		if err == nil {
			err = renameNoReplace(int(staged.directory.Fd()), name, int(filesDirectory.Fd()), candidate)
		}
		if err == nil {
			cleanupInfo = false
		}
		if err == nil {
			return staged.remove(nil)
		}
		if cleanupInfo {
			if cleanupErr := removeTrashInfo(int(infoDirectory.Fd()), infoName, nil); cleanupErr != nil {
				return staged.restore(errors.Join(fmt.Errorf("move %q to system trash: %w", path, err), cleanupErr))
			}
		}
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if errors.Is(err, unix.EXDEV) {
			err = ErrTrashCrossFilesystem
		}
		if errors.Is(err, unix.ENOENT) {
			err = ErrConflict
		}
		return staged.restore(fmt.Errorf("move %q to system trash: %w", path, err))
	}
}

func removeTrashInfo(infoFD int, name string, cause error) error {
	if err := unix.Unlinkat(infoFD, name, 0); err != nil {
		return errors.Join(cause, fmt.Errorf("remove partial trash metadata %q: %w", name, err))
	}
	return cause
}

func homeTrash() (string, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(dataHome) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home trash: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "Trash"), nil
}

func openDirectory(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func createTrashInfo(infoFD int, name string) (*os.File, error) {
	fd, err := unix.Openat(infoFD, name, unix.O_CLOEXEC|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
