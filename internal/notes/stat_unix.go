//go:build darwin || linux

package notes

import (
	"time"

	"golang.org/x/sys/unix"
)

type entryInfo struct {
	directory bool
	regular   bool
	symlink   bool
	size      int64
	modified  time.Time
	identity  FileIdentity
}

func lstatChild(dirFD int, name string) (entryInfo, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(dirFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return entryInfo{}, err
	}
	mode := stat.Mode & unix.S_IFMT
	return entryInfo{directory: mode == unix.S_IFDIR, regular: mode == unix.S_IFREG, symlink: mode == unix.S_IFLNK, size: stat.Size, modified: time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec), identity: identityFromStat(&stat)}, nil
}

func identityFromStat(stat *unix.Stat_t) FileIdentity {
	seconds, nanoseconds := statVersion(stat)
	return FileIdentity{
		Device:             uint64(stat.Dev),
		Inode:              stat.Ino,
		versionSeconds:     seconds,
		versionNanoseconds: nanoseconds,
	}
}
