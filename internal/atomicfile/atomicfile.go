// Package atomicfile puts a finished file where another process reads it, so
// that reader sees the old file or the new one and never half of either, and
// so that after the host itself crashes the disk holds one or the other too.
// spec/StateFile.tla is the model of why each step is there.
package atomicfile

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
)

// Write puts value at path as indented JSON, as WriteFile puts bytes.
func Write(path string, value any, mode fs.FileMode) error {
	text, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(path, append(text, '\n'), mode)
}

// WriteFile puts data at path by renaming a finished file into place. The
// finished file is made beside path, since a rename does not cross
// filesystems, and is on disk before the rename: otherwise the kernel may
// write the rename first, and a host crash leaves path empty. The directory
// is on disk before WriteFile returns, so a write said to be done is not
// undone by a host crash.
func WriteFile(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return err
	}
	return syncDir(dir)
}
