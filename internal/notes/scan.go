package notes

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

func (s *Store) Scan(ctx context.Context) (Snapshot, error) {
	snapshot := Snapshot{Root: s.root, Entries: []Entry{}}
	if err := s.scanDir(ctx, s.rootFD, "", &snapshot.Entries); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

type scannedChild struct {
	directory bool
	entry     fs.DirEntry
	info      entryInfo
}

func (s *Store) scanDir(ctx context.Context, dirFD int, relative RelPath, entries *[]Entry) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	directory, err := duplicateFile(dirFD, scanName(relative))
	if err != nil {
		return fmt.Errorf("open notes directory %q: %w", relative, err)
	}
	defer func() {
		if closeErr := directory.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close notes directory %q: %w", relative, closeErr)
		}
	}()
	dirEntries, err := directory.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("read notes directory %q: %w", relative, err)
	}
	children := make([]scannedChild, 0, len(dirEntries))
	for _, dirEntry := range dirEntries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.skip(dirEntry) {
			continue
		}
		info, statErr := lstatChild(dirFD, dirEntry.Name())
		if statErr != nil {
			if isSymlinkError(statErr) {
				continue
			}
			return fmt.Errorf("stat notes entry %q: %w", joinRelPath(relative, dirEntry.Name()), statErr)
		}
		if info.symlink {
			continue
		}
		if !info.directory && (!info.regular || !strings.EqualFold(filepath.Ext(dirEntry.Name()), ".md")) {
			continue
		}
		children = append(children, scannedChild{directory: info.directory, entry: dirEntry, info: info})
	}
	sort.Slice(children, func(i, j int) bool { return entryLess(children[i], children[j]) })
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := joinRelPath(relative, child.entry.Name())
		kind := KindMarkdown
		if child.directory {
			kind = KindDirectory
		}
		var childDirectory *os.File
		if child.directory {
			var openErr error
			childDirectory, openErr = openScannedDirectory(dirFD, child.entry.Name(), child.info.identity)
			if openErr != nil {
				if isSymlinkError(openErr) || errors.Is(openErr, ErrConflict) {
					continue
				}
				return fmt.Errorf("open notes directory %q: %w", path, openErr)
			}
		}
		*entries = append(*entries, Entry{Path: path, Name: child.entry.Name(), Kind: kind, Size: child.info.size, Modified: child.info.modified, Identity: child.info.identity})
		if child.directory {
			if err := s.scanDir(ctx, int(childDirectory.Fd()), path, entries); err != nil {
				_ = childDirectory.Close()
				return err
			}
			if err := childDirectory.Close(); err != nil {
				return fmt.Errorf("close notes directory %q: %w", path, err)
			}
		}
	}
	return nil
}

func openScannedDirectory(parentFD int, name string, expected FileIdentity) (*os.File, error) {
	directory, err := openChild(parentFD, name, true)
	if err != nil {
		return nil, err
	}
	identity, err := fileIdentity(directory)
	if err == nil && identity != expected {
		err = ErrConflict
	}
	if err != nil {
		if closeErr := directory.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close scanned directory %q: %w", name, closeErr))
		}
		return nil, err
	}
	return directory, nil
}

func scanName(path RelPath) string {
	if path == "" {
		return "."
	}
	return string(path)
}

func entryLess(left, right scannedChild) bool {
	if left.directory != right.directory {
		return left.directory
	}
	if foldedLeft, foldedRight := caseFoldKey(left.entry.Name()), caseFoldKey(right.entry.Name()); foldedLeft != foldedRight {
		return foldedLeft < foldedRight
	}
	return left.entry.Name() < right.entry.Name()
}

func caseFoldKey(name string) string {
	return strings.Map(func(r rune) rune {
		least := r
		for folded := unicode.SimpleFold(r); folded != r; folded = unicode.SimpleFold(folded) {
			if folded < least {
				least = folded
			}
		}
		return least
	}, name)
}

func (s *Store) skip(entry fs.DirEntry) bool {
	name := entry.Name()
	if strings.HasPrefix(name, mutationStagingPrefix) {
		return true
	}
	if _, ignored := s.ignore[name]; ignored {
		return true
	}
	return !s.showHidden && strings.HasPrefix(name, ".")
}
func joinRelPath(parent RelPath, name string) RelPath {
	if parent == "" {
		return RelPath(name)
	}
	return RelPath(string(parent) + "/" + name)
}
