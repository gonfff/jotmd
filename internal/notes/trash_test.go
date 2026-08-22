package notes

import (
	"testing"
	"time"
)

func TestTrashInfoEncodesAbsolutePathAndDeletionTime(t *testing.T) {
	got := trashInfo("/tmp/notes/space #/Привет.md", time.Date(2026, 8, 13, 12, 34, 56, 0, time.UTC))
	want := "[Trash Info]\nPath=/tmp/notes/space%20%23/%D0%9F%D1%80%D0%B8%D0%B2%D0%B5%D1%82.md\nDeletionDate=2026-08-13T12:34:56\n"
	if got != want {
		t.Errorf("trashInfo() = %q, want %q", got, want)
	}
}
