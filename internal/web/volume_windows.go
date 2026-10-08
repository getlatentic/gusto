package web

import "golang.org/x/sys/windows"

// volume is the free and total bytes of the filesystem a path is on.
func volume(path string) (free, total uint64, err error) {
	where, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var available, size, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(where, &available, &size, &totalFree); err != nil {
		return 0, 0, err
	}
	return available, size, nil
}
