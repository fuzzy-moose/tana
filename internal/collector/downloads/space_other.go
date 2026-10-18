//go:build !linux && !darwin && !windows

package downloads

import "errors"

func availableSpace(string) (int64, error) {
	return 0, errors.New("checking available download storage is unsupported on this platform")
}
