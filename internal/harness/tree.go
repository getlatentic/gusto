package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AdapterFile is where a configured harness keeps its adapter. A bare adapter,
// name.json, is read as a directory holding this one file and nothing else.
const AdapterFile = "adapter.json"

// File is one file of a configured harness, as the digest sees it: a slash
// path inside the harness, whether it is executable, and its bytes. Nothing
// else a filesystem knows about a file is part of what the harness is.
type File struct {
	Path       string
	Executable bool
	Content    []byte
}

// Tree is every file of a configured harness, sorted by path byte for byte, so
// two machines holding the same files hold the same tree.
type Tree []File

// ignored names what version control keeps beside the files it tracks. A clone
// and a tarball of the same harness differ only there, and they must digest
// alike, because sharing a harness is a git repository or a tarball.
var ignored = map[string]bool{".git": true, ".hg": true, ".svn": true}

// bare reads name.json as the smallest configured harness: an adapter alone.
func bare(path string) (Tree, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Tree{{Path: AdapterFile, Executable: executable(info.Mode()), Content: text}}, nil
}

// walk reads a configured harness directory. A link or a device is refused by
// name rather than followed: a link reaches outside the harness, so what it
// points at would differ between two machines holding the same directory, and
// carrying it into a job could copy anything on the host.
func walk(root string) (Tree, error) {
	var tree Tree
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ignored[entry.Name()] {
			return skip(entry)
		}
		if entry.IsDir() {
			return nil
		}
		file, err := read(root, path, entry)
		tree = append(tree, file)
		return err
	})
	if err != nil {
		return nil, err
	}
	tree.sort()
	return tree, nil
}

func (t Tree) sort() {
	sort.Slice(t, func(i, j int) bool { return t[i].Path < t[j].Path })
}

func skip(entry fs.DirEntry) error {
	if entry.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

func read(root, path string, entry fs.DirEntry) (File, error) {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return File{}, err
	}
	slashed := filepath.ToSlash(relative)
	if !entry.Type().IsRegular() {
		return File{}, fmt.Errorf("%s in the harness at %s is not a regular file, and a harness holds only those", slashed, root)
	}
	// A newline in a path would let one digest line pass for two.
	if strings.ContainsAny(slashed, "\n\r") {
		return File{}, fmt.Errorf("%q in the harness at %s has a line break in its name", slashed, root)
	}
	info, err := entry.Info()
	if err != nil {
		return File{}, err
	}
	text, err := os.ReadFile(path)
	return File{Path: slashed, Executable: executable(info.Mode()), Content: text}, err
}

func executable(mode fs.FileMode) bool { return mode&0o111 != 0 }

// adapter is the text of the tree's adapter file.
func (t Tree) adapter() ([]byte, bool) {
	for _, file := range t {
		if file.Path == AdapterFile {
			return file.Content, true
		}
	}
	return nil, false
}

// bareOnly says whether the tree is an adapter and nothing else, which is
// carried as name.json, the way a job has always carried one.
func (t Tree) bareOnly() bool {
	return len(t) == 1 && t[0].Path == AdapterFile
}

// writeTo puts the tree under a directory, each file with the mode the digest
// read it as, so a copy digests exactly as the original did.
func (t Tree) writeTo(root string) error {
	for _, file := range t {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, file.Content, file.mode()); err != nil {
			return err
		}
		// WriteFile leaves the mode of a file that was already there.
		if err := os.Chmod(path, file.mode()); err != nil {
			return err
		}
	}
	return nil
}

func (f File) mode() fs.FileMode {
	if f.Executable {
		return 0o755
	}
	return 0o644
}

// errBoth is a harness offered twice in one place, which Open will not choose
// between: whichever it picked, somebody meant the other.
var errBoth = errors.New("is both a file and a directory")
