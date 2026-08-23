//go:build linux

package notes

import "golang.org/x/sys/unix"

func statVersion(*unix.Stat_t) (int64, int64) {
	return 0, 0
}
