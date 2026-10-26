package roomlog

import (
	"errors"
	"io/fs"
	"os"
)

// appendLine adds one line to a log's file in a single write, so a crash leaves a whole line or a torn last one, which a
// reader drops.
func appendLine(path string, line []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(line); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// replaceFile writes a log's file whole, beside the old one, and moves it into place.
func replaceFile(path string, content []byte) error {
	scratch := path + ".new"
	file, err := os.OpenFile(scratch, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(scratch)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(scratch)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(scratch)
		return err
	}
	return os.Rename(scratch, path)
}

func removeLog(path string) error {
	for _, each := range []string{path, path + ".new"} {
		if err := os.Remove(each); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
