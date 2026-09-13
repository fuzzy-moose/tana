package downloads

import (
	"math"

	"golang.org/x/sys/windows"
)

func availableSpace(path string) (int64, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	if err := windows.GetDiskFreeSpaceEx(name, &available, nil, nil); err != nil {
		return 0, err
	}
	return int64(min(available, math.MaxInt64)), nil
}
