//go:build darwin || linux

package notes

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func openRoot(path string) (int, error) {
	return unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
}
func duplicateFile(fd int, name string) (*os.File, error) {
	duplicate, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(duplicate), name), nil
}
func duplicateDescriptor(file *os.File) (int, error) { return unix.Dup(int(file.Fd())) }
func openChild(dirFD int, name string, directory bool) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(dirFD, name, flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
func openRelative(rootFD int, path RelPath, directory bool) (*os.File, error) {
	parts, err := relativeParts(path)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Dup(rootFD)
	if err != nil {
		return nil, err
	}
	for index, part := range parts {
		child, openErr := openChild(fd, part, index < len(parts)-1 || directory)
		closeErr := unix.Close(fd)
		if openErr != nil {
			return nil, openErr
		}
		if closeErr != nil {
			_ = child.Close()
			return nil, closeErr
		}
		if index == len(parts)-1 {
			return child, nil
		}
		fd, err = duplicateDescriptor(child)
		if err != nil {
			_ = child.Close()
			return nil, err
		}
		if err := child.Close(); err != nil {
			return nil, err
		}
	}
	return nil, errors.New("empty relative path")
}
func relativeParts(path RelPath) ([]string, error) {
	raw := string(path)
	if raw == "" || strings.ContainsRune(raw, '\x00') || strings.HasPrefix(raw, "/") {
		return nil, fmt.Errorf("invalid relative path %q", path)
	}
	parts := strings.Split(raw, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, fmt.Errorf("invalid relative path %q", path)
		}
	}
	return parts, nil
}
func isSymlinkError(err error) bool { return errors.Is(err, unix.ELOOP) }
