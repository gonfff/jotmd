//go:build darwin || linux

package notes

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

var (
	ErrConflict = errors.New("filesystem entry changed")
	ErrExists   = errors.New("target already exists")
)

func checkedChild(dirFD int, name string, expected FileIdentity, allowSymlink bool) (entryInfo, error) {
	info, err := lstatChild(dirFD, name)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return entryInfo{}, ErrConflict
		}
		return entryInfo{}, err
	}
	if info.identity != expected || info.symlink && !allowSymlink {
		return entryInfo{}, ErrConflict
	}
	return info, nil
}

func fileIdentity(file *os.File) (FileIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return FileIdentity{}, fmt.Errorf("stat opened entry: %w", err)
	}
	return identityFromStat(&stat), nil
}

func mutationParent(path RelPath) (RelPath, string, error) {
	parts, err := relativeParts(path)
	if err != nil {
		return "", "", err
	}
	name := parts[len(parts)-1]
	if len(parts) == 1 {
		return "", name, nil
	}
	parent := RelPath(parts[0])
	for _, part := range parts[1 : len(parts)-1] {
		parent = joinRelPath(parent, part)
	}
	return parent, name, nil
}
