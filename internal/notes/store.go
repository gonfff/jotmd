package notes

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Kind uint8

const (
	KindDirectory Kind = iota
	KindMarkdown
)

type FileIdentity struct {
	Device uint64
	Inode  uint64
}

type Entry struct {
	Path     RelPath
	Name     string
	Kind     Kind
	Size     int64
	Modified time.Time
	Identity FileIdentity
}

type Snapshot struct {
	Root    string
	Entries []Entry
}

type Store struct {
	root         string
	rootFD       int
	ignore       map[string]struct{}
	readMaxBytes int64
	showHidden   bool
}

func NewStore(root string, ignore []string, showHidden bool) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("notes root must not be empty")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve notes root %q: %w", root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat notes root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("notes root %q is not a directory", root)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve notes root %q: %w", root, err)
	}
	ignored := make(map[string]struct{}, len(ignore))
	for _, name := range ignore {
		ignored[name] = struct{}{}
	}
	rootFD, err := openRoot(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("open notes root %q: %w", root, err)
	}
	return &Store{
		root:         absRoot,
		rootFD:       rootFD,
		ignore:       ignored,
		readMaxBytes: DefaultReadMaxBytes,
		showHidden:   showHidden,
	}, nil
}
