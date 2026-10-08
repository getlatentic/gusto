package mirror

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// scanned is a local file as the last look found it, with what it takes to tell later whether it is still that file.
type scanned struct {
	Entry
	modifiedNs int64
	executable bool
}

// Skipped is what a look left out that a person may want to know of: it is not kept in step, and nothing is done to it.
type Skipped struct {
	// Links are symbolic links, which are not followed and not copied: a link is a place on one machine.
	Links []string
	// Large are files over the size limit, on either side. They are left as they are.
	Large []string
	// Ignored is how many paths the ignore rules left out.
	Ignored int
}

// hashFile is the sha256 of a file's content.
func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// scanLocal is the files of the local folder: not the ignored, not links, not those too large. A file whose size and time are
// what they were when it was hashed is not read again.
func (e *Engine) scanLocal(skipped *Skipped) (map[string]scanned, error) {
	found := map[string]scanned{}
	err := filepath.WalkDir(e.Root, func(full string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return nil
			}
			return err
		}
		relative, err := filepath.Rel(e.Root, full)
		if err != nil || relative == "." {
			return nil
		}
		name := filepath.ToSlash(relative)
		if e.Ignore.Skips(name, entry.IsDir()) {
			skipped.Ignored++
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			skipped.Links = append(skipped.Links, name)
			return nil
		case entry.IsDir() || !entry.Type().IsRegular():
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if info.Size() > e.mostBytes() {
			skipped.Large = append(skipped.Large, name)
			return nil
		}
		hash, err := e.localHash(name, full, info)
		if err != nil {
			return nil
		}
		found[name] = scanned{Entry: Entry{Hash: hash, Size: info.Size()}, modifiedNs: info.ModTime().UnixNano(), executable: info.Mode()&0o111 != 0}
		return nil
	})
	return found, err
}

func (e *Engine) localHash(name, full string, info fs.FileInfo) (string, error) {
	if known, ok := e.State.Local[name]; ok && known.Size == info.Size() && known.ModifiedNs == info.ModTime().UnixNano() {
		return known.Hash, nil
	}
	hash, err := hashFile(full)
	if err != nil {
		return "", err
	}
	e.State.Local[name] = Stamp{Hash: hash, Size: info.Size(), ModifiedNs: info.ModTime().UnixNano()}
	return hash, nil
}

// onDisk is where a path of the folder is on this computer, when it is somewhere a sync may put a file. The path came from
// another machine, so it is made a path of this one by the rules of this one, and none of the places it goes through may be a
// link, which could lead out of the folder.
func (e *Engine) onDisk(name string) (string, error) {
	local, err := filepath.Localize(name)
	if err != nil || name == "" || strings.HasPrefix(name, "/") {
		return "", errors.New(name + " is not a name this computer can keep a file under")
	}
	full := e.Root
	parts := strings.Split(local, string(os.PathSeparator))
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errors.New(name + " is not a path inside the folder")
		}
		full = filepath.Join(full, part)
		info, err := os.Lstat(full)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return filepath.Join(append([]string{e.Root}, parts...)...), nil
		case err != nil:
			return "", err
		case info.Mode()&fs.ModeSymlink != 0:
			return "", errors.New(name + " goes through a link, which is not followed")
		case i < len(parts)-1 && !info.IsDir():
			return "", errors.New(name + " goes through a file")
		}
	}
	return full, nil
}
