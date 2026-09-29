package main

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// replaceTree makes to hold what from holds, and nothing else. The copy is made beside
// to and swapped in, so a copy that fails part way leaves what was there. Links are
// copied as links and never followed: what an agent linked to on this machine is not
// read, and a workspace that is copied back has nothing in it that was not the agent's.
func replaceTree(from, to string) error {
	next, old := to+".next", to+".old"
	for _, leftover := range []string{next, old} {
		if err := os.RemoveAll(leftover); err != nil {
			return err
		}
	}
	if err := copyTree(from, next); err != nil {
		os.RemoveAll(next)
		return err
	}
	if _, err := os.Lstat(to); err == nil {
		if err := os.Rename(to, old); err != nil {
			os.RemoveAll(next)
			return err
		}
	}
	if err := os.Rename(next, to); err != nil {
		os.Rename(old, to)
		return err
	}
	return os.RemoveAll(old)
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode().Perm())
		}
		return nil
	})
}

func copyFile(from, to string, mode fs.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode|0o200)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
