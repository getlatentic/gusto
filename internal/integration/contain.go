package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The most a plugin may hold. A plugin comes from a stranger, and these keep reading it from costing more than a plugin is worth.
const (
	MaxFiles     = 5000
	MaxBytes     = 100 << 20
	MaxDepth     = 12
	maxManifest  = 1 << 20
	maxSkillFile = 1 << 20
)

// within says whether a path, once its links are followed, is the root or under it. Both are made real first.
func within(root, path string) bool {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return false
	}
	relative, err := filepath.Rel(root, real)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// regular says whether a path is, after links, a plain file under the root.
func regular(root, path string) bool {
	if !within(root, path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func directory(root, path string) bool {
	if !within(root, path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// measure says how much a tree holds, and the first link in it that leaves the root, which is not a thing a plugin may have.
func measure(root, top string) (files int, bytes int64, escape string, err error) {
	err = filepath.WalkDir(top, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, _ := filepath.Rel(root, path)
		if depth := strings.Count(relative, string(filepath.Separator)); depth > MaxDepth {
			return rejected("%s is nested more than %d folders deep", relative, MaxDepth)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			if !within(root, path) && escape == "" {
				escape = relative
			}
			return nil
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			files++
			bytes += info.Size()
		}
		return nil
	})
	return files, bytes, escape, err
}
