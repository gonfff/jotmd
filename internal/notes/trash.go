package notes

import (
	"errors"
	"net/url"
	"time"
)

var ErrTrashCrossFilesystem = errors.New("system trash is on another filesystem")
var ErrTrashPermissionDenied = errors.New("system trash permission denied")

func trashInfo(absolutePath string, deleted time.Time) string {
	encoded := (&url.URL{Path: absolutePath}).EscapedPath()
	return "[Trash Info]\nPath=" + encoded + "\nDeletionDate=" + deleted.Format("2006-01-02T15:04:05") + "\n"
}
