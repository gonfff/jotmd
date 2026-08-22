//go:build darwin

package notes

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"
)

const finderTrashScript = "on run argv\ntell application \"Finder\" to delete POSIX file (item 1 of argv)\nend run"

func (s *Store) Trash(ctx context.Context, path RelPath, expected FileIdentity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	parent, name, err := mutationParent(path)
	if err != nil {
		return err
	}
	directory, err := s.openParent(parent)
	if err != nil {
		return fmt.Errorf("open trash parent %q: %w", parent, err)
	}
	defer directory.Close()
	staged, err := stageCheckedEntry(int(directory.Fd()), name, expected)
	if err != nil {
		return fmt.Errorf("trash %q: %w", path, err)
	}
	defer staged.Close()
	return finishStagedTrash(ctx, path, expected, staged)
}

func finishStagedTrash(ctx context.Context, path RelPath, expected FileIdentity, staged *stagedEntry) error {
	if _, err := checkedChild(int(staged.directory.Fd()), staged.name, expected, false); err != nil {
		return staged.restore(fmt.Errorf("verify staged entry for %q: %w", path, err))
	}
	stagedPath := filepath.Join(staged.path, staged.name)
	if err := runFinderTrash(ctx, stagedPath); err != nil {
		return staged.restore(fmt.Errorf("move %q to system trash: %w", path, err))
	}
	return staged.remove(nil)
}

var runFinderTrash = func(ctx context.Context, path string) error {
	return finderTrashCommand(ctx, path).Run()
}

func finderTrashCommand(ctx context.Context, path string) *exec.Cmd {
	return exec.CommandContext(ctx, "/usr/bin/osascript", "-e", finderTrashScript, "--", path)
}

func descriptorPath(fd int) (string, error) {
	const darwinPathMax = 1024
	buffer := make([]byte, darwinPathMax)
	_, _, errno := unix.Syscall(unix.SYS_FCNTL, uintptr(fd), unix.F_GETPATH, uintptr(unsafe.Pointer(&buffer[0])))
	if errno != 0 {
		return "", errno
	}
	if end := bytes.IndexByte(buffer, 0); end >= 0 {
		buffer = buffer[:end]
	}
	return string(buffer), nil
}
