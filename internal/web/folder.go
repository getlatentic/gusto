package web

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/pkg/task"
)

// changed is the files a turn changed, from its patch, for the chat to link to.
func changed(dir task.Dir) []string {
	names, _ := changes(dir)
	return names
}

// changes is every file a turn left different, and of those the ones it deleted.
func changes(dir task.Dir) (names, deleted []string) {
	if !dir.PlainOut("diff.patch") {
		return nil, nil
	}
	patch, err := os.ReadFile(dir.OutFile("diff.patch"))
	if err != nil {
		return nil, nil
	}
	for _, file := range parseDiff(string(patch)) {
		names = append(names, file.Name)
		if file.Deleted {
			deleted = append(deleted, file.Name)
		}
	}
	return names, deleted
}

// mostShown is how much of a file the viewer draws. A folder can hold
// anything, and a page is not the place to read a gigabyte.
const mostShown = 512 << 10

// resolveInFolder resolves a path inside a folder, and refuses it when it leaves it, by a link or otherwise, or reaches
// into git's own directory. The path is as the person asked, cleaned and rooted.
func resolveInFolder(folder, asked string) (string, bool) {
	root, err := filepath.EvalSymlinks(folder)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, asked))
	if err != nil || !strings.HasPrefix(resolved, root+string(filepath.Separator)) || strings.Contains(asked, "/.git/") || strings.HasPrefix(asked, "/.git") {
		return "", false
	}
	return resolved, true
}

func readSome(path string, most int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	text := make([]byte, most)
	read, err := file.Read(text)
	if err != nil && read == 0 {
		return nil, fs.ErrNotExist
	}
	return text[:read], nil
}
