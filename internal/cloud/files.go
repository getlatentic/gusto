package cloud

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// writeFile puts a finished file at path by renaming it into place, so a reader
// sees the old file or the new one and never half of either.
func writeFile(path string, value any, mode fs.FileMode) error {
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

// createFile is writeFile that fails, with fs.ErrExist, when the file is there:
// a finished file is linked into place, which is one step that either claims the
// name or does not. Two callers cannot both have made it.
func createFile(path string, value any, mode fs.FileMode) error {
	text, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".create-")
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
	return os.Link(temp.Name(), path)
}

func readFile(path string, into any) error {
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(text, into)
}

func missing(err error) bool { return errors.Is(err, fs.ErrNotExist) }

// readDir is the names in a directory.
func readDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names, nil
}
