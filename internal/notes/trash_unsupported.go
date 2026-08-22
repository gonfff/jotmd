//go:build !darwin && !linux

package notes

import (
	"context"
	"errors"
)

func (s *Store) Trash(context.Context, RelPath, FileIdentity) error {
	return errors.New("system trash is unsupported on this platform")
}
