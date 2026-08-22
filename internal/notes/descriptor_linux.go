//go:build linux

package notes

import (
	"os"
	"strconv"
)

func descriptorPath(fd int) (string, error) {
	return os.Readlink("/proc/self/fd/" + strconv.Itoa(fd))
}
