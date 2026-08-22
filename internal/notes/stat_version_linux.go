//go:build linux

package notes

import "golang.org/x/sys/unix"

func statVersion(*unix.Stat_t) (int64, int64) {
	// ponytail: Linux keeps device/inode identity until Linux releases return; use statx birth time then.
	return 0, 0
}
