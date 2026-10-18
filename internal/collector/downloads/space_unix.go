//go:build linux || darwin

package downloads

import (
	"math"

	"golang.org/x/sys/unix"
)

func availableSpace(path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	// Bavail excludes blocks reserved for other users, unlike Bfree.
	blocks, size := uint64(stat.Bavail), uint64(stat.Bsize)
	if size != 0 && blocks > math.MaxInt64/size {
		return math.MaxInt64, nil
	}
	return int64(blocks * size), nil
}
