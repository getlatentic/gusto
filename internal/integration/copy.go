package integration

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// copyTree copies the kept files of a plugin into a folder that does not exist yet. A link to a file in the plugin becomes the file.
// The copy holds no link, so nothing in it can point out of it, and a file is runnable in the copy when it was in the plugin.
func copyTree(root, from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !kept(root, path, entry) {
			return nil
		}
		relative, _ := filepath.Rel(from, path)
		return copyFile(path, filepath.Join(to, relative))
	})
}

func copyFile(from, to string) error {
	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if info.Mode()&0o111 != 0 {
		mode = 0o755
	}
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		target.Close()
		return err
	}
	if err := target.Close(); err != nil {
		return err
	}
	return os.Chmod(to, mode)
}
