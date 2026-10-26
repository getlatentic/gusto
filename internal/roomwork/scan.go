package roomwork

import (
	"io/fs"
	"os"
	"path/filepath"
)

// kind is what an entry in a folder is. Folders are not entries: a folder exists because a file is in it.
type kind uint8

const (
	file kind = iota + 1
	link
)

// stamp is what the file system numbers a file by. The change time moves with any write, rename or permission change, which no
// program can set back, and a file replaced by a rename is a new inode. A platform that gives neither leaves it empty.
type stamp struct {
	change int64
	inode  uint64
}

// entry is what is known of one file without reading it. Two entries that match mean the file was not written between them.
type entry struct {
	kind  kind
	mode  fs.FileMode
	size  int64
	mtime int64
	stamp
}

// tree is the files under a folder by their path from it.
type tree map[string]entry

// kept is the name of a repository's own bookkeeping. A working copy carries it so the agent can use git, and a merge leaves it
// alone: two copies of one repository cannot be merged file by file, and a turn's commits are not the folder's.
const kept = ".git"

func entryOf(info fs.FileInfo) (entry, bool) {
	switch {
	case info.Mode().IsRegular():
		return entry{file, info.Mode().Perm(), info.Size(), info.ModTime().UnixNano(), stampOf(info)}, true
	case info.Mode()&fs.ModeSymlink != 0:
		return entry{kind: link, mtime: info.ModTime().UnixNano(), stamp: stampOf(info)}, true
	}
	return entry{}, false
}

// scan lists the files under root, a repository's bookkeeping and anything that is not a file or a link left out. Links are
// entries and are never followed.
func scan(root string) (tree, error) {
	found := tree{}
	err := filepath.WalkDir(root, func(path string, each fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if each.Name() == kept {
			if each.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if each.IsDir() {
			return nil
		}
		info, err := each.Info()
		if err != nil {
			return err
		}
		if made, ok := entryOf(info); ok {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			found[filepath.ToSlash(relative)] = made
		}
		return nil
	})
	return found, err
}

// lookup is the entry at a path in a folder, and whether there is a file or link there.
func lookup(folder, relative string) (entry, bool) {
	return lookupFile(filepath.Join(folder, filepath.FromSlash(relative)))
}

func lookupFile(path string) (entry, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return entry{}, false
	}
	return entryOf(info)
}

// unchanged says whether two entries of one path describe a file nobody wrote between them.
func unchanged(a, b entry) bool {
	return a.kind == b.kind && a.mode == b.mode && a.size == b.size && a.mtime == b.mtime && a.stamp == b.stamp
}
