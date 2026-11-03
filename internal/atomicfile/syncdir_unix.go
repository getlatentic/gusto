//go:build !windows

package atomicfile

import "os"

// syncDir puts a directory's names on disk: the rename into it.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
