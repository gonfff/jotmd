//go:build linux

package notes

import "golang.org/x/sys/unix"

func renameNoReplace(fromFD int, oldName string, toFD int, newName string) error {
	return unix.Renameat2(fromFD, oldName, toFD, newName, unix.RENAME_NOREPLACE)
}
