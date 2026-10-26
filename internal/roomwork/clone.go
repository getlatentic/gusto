package roomwork

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Limits bound what one working copy may take: the bytes and the files of the folder it is made from. A copy over a limit is
// refused, because every turn that runs beside another holds a copy of its own.
type Limits struct {
	Bytes int64
	Files int
}

// TooBigError says a folder is over the limits for a working copy.
type TooBigError struct{ Limits Limits }

func (e *TooBigError) Error() string {
	return fmt.Sprintf("this room's folder is more than a turn can work on a copy of (%d MB or %d files); take files out of it, or raise GUSTO_ROOM_COPY_MB and GUSTO_ROOM_COPY_FILES",
		e.Limits.Bytes>>20, e.Limits.Files)
}

// checkEvery is how many files are copied between looks at the context.
const checkEvery = 256

// clone copies the folder from to the existing folder to, repository bookkeeping included, and says what it found in from. A
// copy shares the data blocks of the original where the file system can do that, so it costs the file count and not the bytes.
// A copy that fails is removed.
func clone(ctx context.Context, from, to string, limits Limits) (tree, error) {
	found := tree{}
	var bytes int64
	var files int
	err := filepath.WalkDir(from, func(path string, each fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if files%checkEvery == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		info, err := each.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if each.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		made, ok := entryOf(info)
		if !ok {
			return nil
		}
		files++
		bytes += info.Size()
		if bytes > limits.Bytes || files > limits.Files {
			return &TooBigError{limits}
		}
		if err := place(path, target, info); err != nil {
			return err
		}
		if !insideKept(relative) {
			found[filepath.ToSlash(relative)] = made
		}
		return nil
	})
	if err != nil {
		_ = os.RemoveAll(to)
	}
	return found, err
}

// CopyTree copies the folder from to the existing folder to, as a working copy is made. It is how an agent's state is seeded.
func CopyTree(ctx context.Context, from, to string, limits Limits) error {
	_, err := clone(ctx, from, to, limits)
	return err
}

// insideKept says whether a path from a folder is a repository's bookkeeping or inside it.
func insideKept(relative string) bool {
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		if part == kept {
			return true
		}
	}
	return false
}

// place puts a copy of a file or link at to, keeping its permissions and modified time.
func place(from, to string, info fs.FileInfo) error {
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(target, to)
	}
	if err := cloneFile(from, to); err != nil {
		if err := copyBytes(from, to, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return os.Chtimes(to, info.ModTime(), info.ModTime())
}

func copyBytes(from, to string, mode fs.FileMode) error {
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
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(to, mode)
}
