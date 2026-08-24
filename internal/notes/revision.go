package notes

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	ErrInvalidRevision  = errors.New("invalid revision")
	ErrRevisionConflict = errors.New("revision conflict")
	ErrRecoveryRequired = errors.New("filesystem recovery required")
)

type RevisionConflictError struct {
	Expected Revision
	Actual   Revision
}

func (e *RevisionConflictError) Error() string {
	if e.Actual == "" {
		return fmt.Sprintf("%s: expected %s, note does not exist", ErrRevisionConflict, e.Expected)
	}
	return fmt.Sprintf("%s: expected %s, found %s", ErrRevisionConflict, e.Expected, e.Actual)
}

func (e *RevisionConflictError) Unwrap() error {
	return ErrRevisionConflict
}

func ParseRevision(value string) (Revision, error) {
	encoded, ok := strings.CutPrefix(value, "sha256:")
	if !ok || len(encoded) != sha256.Size*2 {
		return "", fmt.Errorf("%w: expected sha256 followed by 64 hexadecimal digits", ErrInvalidRevision)
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidRevision, err)
	}
	return Revision("sha256:" + hex.EncodeToString(decoded)), nil
}

func revisionBytes(content []byte) Revision {
	sum := sha256.Sum256(content)
	return Revision("sha256:" + hex.EncodeToString(sum[:]))
}

func revisionFile(file *os.File) (Revision, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek note for revision: %w", err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("read note for revision: %w", err)
	}
	return Revision("sha256:" + hex.EncodeToString(hash.Sum(nil))), nil
}
