package notes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

type Revision string

type Document struct {
	Path     RelPath
	Content  []byte
	Revision Revision
	Modified time.Time
}

const (
	DefaultReadMaxBytes int64 = 2 * 1024 * 1024
	maxReadBytes        int64 = 64 * 1024 * 1024
)

var ErrNoteTooLarge = errors.New("note exceeds preview size limit")

type TooLargeError struct {
	Size  int64
	Limit int64
}

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("%s: %d bytes exceeds %d byte limit", ErrNoteTooLarge, e.Size, e.Limit)
}

func (e *TooLargeError) Unwrap() error {
	return ErrNoteTooLarge
}

func (s *Store) SetReadMaxBytes(maxBytes int64) error {
	if maxBytes < 1 || maxBytes > maxReadBytes {
		return fmt.Errorf("preview.max_bytes must be between 1 and %d", maxReadBytes)
	}
	s.readMaxBytes = maxBytes
	return nil
}

func (s *Store) Read(ctx context.Context, path RelPath) (document Document, err error) {
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	file, err := openRelative(s.rootFD, path, false)
	if err != nil {
		return Document{}, fmt.Errorf("open note %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			document = Document{}
			err = fmt.Errorf("close note %q: %w", path, closeErr)
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return Document{}, fmt.Errorf("stat note %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return Document{}, fmt.Errorf("note %q is not a regular file", path)
	}
	if info.Size() > s.readMaxBytes {
		return Document{}, &TooLargeError{Size: info.Size(), Limit: s.readMaxBytes}
	}
	content, err := io.ReadAll(io.LimitReader(file, s.readMaxBytes+1))
	if err != nil {
		return Document{}, fmt.Errorf("read note %q: %w", path, err)
	}
	if int64(len(content)) > s.readMaxBytes {
		size := int64(len(content))
		if latest, statErr := file.Stat(); statErr == nil && latest.Mode().IsRegular() {
			size = latest.Size()
		}
		return Document{}, &TooLargeError{Size: size, Limit: s.readMaxBytes}
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	sum := sha256.Sum256(content)
	return Document{
		Path:     path,
		Content:  content,
		Revision: Revision("sha256:" + hex.EncodeToString(sum[:])),
		Modified: info.ModTime(),
	}, nil
}
