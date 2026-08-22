//go:build darwin

package notes

import "golang.org/x/sys/unix"

func statVersion(stat *unix.Stat_t) (int64, int64) {
	return stat.Btim.Sec, stat.Btim.Nsec
}
