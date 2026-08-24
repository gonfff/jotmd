package notes

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"
	"unicode"

	"golang.org/x/sys/unix"
)

func (s *Store) CreateNote(ctx context.Context, parent RelPath, title string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	filename, err := Filename(title)
	if err != nil {
		return Document{}, err
	}
	parentDirectory, err := s.openParent(parent)
	if err != nil {
		return Document{}, fmt.Errorf("open note parent %q: %w", parent, err)
	}
	defer parentDirectory.Close()

	stem := strings.TrimSuffix(filename, ".md")
	for number := 1; ; number++ {
		if err := ctx.Err(); err != nil {
			return Document{}, err
		}
		candidate := filename
		if number > 1 {
			candidate = fmt.Sprintf("%s-%d.md", stem, number)
		}
		file, err := createFile(int(parentDirectory.Fd()), candidate)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return Document{}, fmt.Errorf("create note %q: %w", joinRelPath(parent, candidate), err)
		}
		if err := closeCreatedNote(file, joinRelPath(parent, candidate)); err != nil {
			return Document{}, err
		}
		return Document{
			Path:     joinRelPath(parent, candidate),
			Content:  []byte{},
			Revision: revisionBytes(nil),
			Modified: time.Now(),
		}, nil
	}
}

func closeCreatedNote(file *os.File, path RelPath) error {
	if err := file.Close(); err != nil {
		return fmt.Errorf("close created note %q: %w", path, err)
	}
	return nil
}

func (s *Store) CreateDirectory(ctx context.Context, parent RelPath, name string) (Entry, error) {
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	if err := validDirectoryName(name); err != nil {
		return Entry{}, err
	}
	parentDirectory, err := s.openParent(parent)
	if err != nil {
		return Entry{}, fmt.Errorf("open directory parent %q: %w", parent, err)
	}
	defer parentDirectory.Close()
	if err := unix.Mkdirat(int(parentDirectory.Fd()), name, 0o777); err != nil {
		return Entry{}, fmt.Errorf("create directory %q: %w", joinRelPath(parent, name), err)
	}
	info, err := lstatChild(int(parentDirectory.Fd()), name)
	if err != nil {
		return Entry{}, fmt.Errorf("stat created directory %q: %w", joinRelPath(parent, name), err)
	}
	return Entry{Path: joinRelPath(parent, name), Name: name, Kind: KindDirectory, Size: info.size, Modified: info.modified, Identity: info.identity}, nil
}

func (s *Store) openParent(parent RelPath) (*os.File, error) {
	if parent == "" {
		return duplicateFile(s.rootFD, ".")
	}
	return openRelative(s.rootFD, parent, true)
}

func createFile(parentFD int, name string) (*os.File, error) {
	fd, err := unix.Openat(parentFD, name, unix.O_CLOEXEC|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_WRONLY, 0o666)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func validDirectoryName(name string) error {
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, mutationStagingPrefix) || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("invalid directory name %q", name)
	}
	for _, runeValue := range name {
		if unicode.IsControl(runeValue) {
			return fmt.Errorf("invalid directory name %q", name)
		}
	}
	return nil
}
