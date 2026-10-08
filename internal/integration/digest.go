package integration

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// digest is a fingerprint of a folder's files: their paths and what is in them, and whether they can be run. Two folders with the
// same files have the same digest, whatever their dates or where they are.
func digest(root, top string) (string, error) {
	var files []string
	err := filepath.WalkDir(top, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if kept(root, path, entry) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, path := range files {
		relative, _ := filepath.Rel(top, path)
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		io.WriteString(hash, filepath.ToSlash(relative)+"\x00")
		if info.Mode()&0o111 != 0 {
			io.WriteString(hash, "x")
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, err = io.Copy(hash, file)
		file.Close()
		if err != nil {
			return "", err
		}
		io.WriteString(hash, "\x00")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// kept says whether a path in a plugin is a file that is kept: a file, or a link to a file that is inside the plugin. Version
// control's own folder is not kept, and a link that leaves the plugin is not followed.
func kept(root, path string, entry fs.DirEntry) bool {
	if entry.IsDir() {
		return false
	}
	if entry.Name() == ".git" || inGit(root, path) {
		return false
	}
	return regular(root, path)
}

func inGit(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return true
	}
	for dir := filepath.Dir(relative); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if filepath.Base(dir) == ".git" {
			return true
		}
	}
	return false
}
