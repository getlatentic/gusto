// Package atomicfile puts a finished file where another process reads it, so
// that reader sees the old file or the new one and never half of either.
package atomicfile

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
)

// Write puts value at path as indented JSON, by renaming a finished file into
// place. The finished file is made beside path, since a rename does not cross
// filesystems.
func Write(path string, value any, mode fs.FileMode) error {
	text, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(text, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
