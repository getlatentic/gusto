package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// A copy is kept while it is one of the newest few or younger than a day, so a server still running an older one, on a
// host that has not been told yet, is not left with its program removed from under it.
const (
	keptReleases = 3
	keptFor      = 24 * time.Hour
)

// ShareRelease copies the program at exe into dir, under a name made from what it holds, and says where the copy is.
// Hosts apart run organizations' servers from there, so a server is the front door's release and not the one its host
// was made with. Copies of older releases are removed past the newest few.
func ShareRelease(exe, dir string) (string, error) {
	sum, err := fileSum(exe)
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, sum, "gusto")
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	if err := copyExecutable(exe, target); err != nil {
		return "", err
	}
	pruneReleases(dir, sum, time.Now())
	return target, nil
}

func fileSum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil))[:16], nil
}

// copyExecutable writes the copy beside where it goes and renames it there, so a host never runs half of one.
func copyExecutable(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	temp, err := os.CreateTemp(filepath.Dir(to), ".gusto-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := io.Copy(temp, source); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o755); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), to)
}

// pruneReleases removes the copies of releases other than current that are neither among the newest few nor younger
// than a day. A copy that cannot be removed is tried again at the next release.
func pruneReleases(dir, current string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type release struct {
		name    string
		changed time.Time
	}
	var found []release
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !entry.IsDir() || entry.Name() == current {
			continue
		}
		found = append(found, release{entry.Name(), info.ModTime()})
	}
	sort.Slice(found, func(a, b int) bool { return found[a].changed.After(found[b].changed) })
	// The current release is one of the newest few, so as many others are kept as leave room for it.
	for i, old := range found {
		if i+1 >= keptReleases && now.Sub(old.changed) >= keptFor {
			os.RemoveAll(filepath.Join(dir, old.name))
		}
	}
}
