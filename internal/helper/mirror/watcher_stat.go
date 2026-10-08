package mirror

import "os"

// statDir says whether a path is a folder, without following a link.
func statDir(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}
