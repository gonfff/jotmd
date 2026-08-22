//go:build darwin

package notes

import "golang.org/x/sys/unix"

func renameNoReplace(fromFD int, oldName string, toFD int, newName string) error {
	return unix.RenameatxNp(fromFD, oldName, toFD, newName, unix.RENAME_EXCL)
}
