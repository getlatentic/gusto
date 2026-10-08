package integration

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// MaxUpload is the most an uploaded archive may be, before it is unpacked.
const MaxUpload = 50 << 20

// Unpack unpacks an uploaded zip or .tar.gz into a new folder under parent, and returns the folder to read. The archive comes from a
// stranger, so it is unpacked by these rules and never by a tool's own: a name that is absolute or climbs is refused, a link is
// refused, a file that would be written twice is refused, and the files, the folders and the bytes written are counted as they are
// written, so an archive that is small and unpacks to very much stops at the limit.
func Unpack(data []byte, parent string) (string, error) {
	return unpack(data, parent, MaxFiles, MaxBytes)
}

func unpack(data []byte, parent string, maxFiles int, maxBytes int64) (string, error) {
	if len(data) > MaxUpload {
		return "", rejected("the upload is larger than %d MiB", MaxUpload>>20)
	}
	into, err := os.MkdirTemp(parent, "unpack-")
	if err != nil {
		return "", err
	}
	sink := &sink{root: into, maxFiles: maxFiles, maxBytes: maxBytes}
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		err = sink.zip(data)
	case bytes.HasPrefix(data, []byte{0x1f, 0x8b}):
		err = sink.tarGz(data)
	default:
		err = rejected("the upload is not a .zip or a .tar.gz")
	}
	if err != nil {
		os.RemoveAll(into)
		return "", err
	}
	top, err := pluginFolder(into)
	if err != nil {
		os.RemoveAll(into)
		return "", err
	}
	return top, nil
}

// pluginFolder is the folder that holds plugin.json: the one unpacked into, or the one folder an archive made from a repository
// puts everything in.
func pluginFolder(into string) (string, error) {
	if _, err := os.Lstat(filepath.Join(into, "plugin.json")); err == nil {
		return into, nil
	}
	entries, err := os.ReadDir(into)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		return filepath.Join(into, entries[0].Name()), nil
	}
	return into, nil
}

type sink struct {
	root     string
	maxFiles int
	maxBytes int64
	files    int
	written  int64
	seen     map[string]bool
}

// target is where an archive's name is written, or an error for a name that is not safe.
func (s *sink) target(name string) (string, error) {
	slashed := strings.ReplaceAll(name, "\\", "/")
	clean := path.Clean(slashed)
	if name == "" || strings.ContainsRune(name, 0) || path.IsAbs(slashed) || clean == ".." || strings.HasPrefix(clean, "../") || (len(slashed) > 1 && slashed[1] == ':') {
		return "", rejected("%q is not a name inside the plugin", name)
	}
	if strings.Count(clean, "/") > MaxDepth {
		return "", rejected("%q is nested more than %d folders deep", name, MaxDepth)
	}
	return filepath.Join(s.root, filepath.FromSlash(clean)), nil
}

func (s *sink) file(name string, mode os.FileMode, from io.Reader) error {
	target, err := s.target(name)
	if err != nil {
		return err
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	if s.seen[target] {
		return rejected("%q is in the archive twice", name)
	}
	s.seen[target] = true
	if s.files++; s.files > s.maxFiles {
		return rejected("it holds more than %d files", s.maxFiles)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	room := s.maxBytes - s.written
	n, err := io.Copy(out, io.LimitReader(from, room+1))
	s.written += n
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && s.written > s.maxBytes {
		err = rejected("it unpacks to more than %d MiB", s.maxBytes>>20)
	}
	if err == nil {
		err = os.Chmod(target, perm)
	}
	return err
}

func (s *sink) folder(name string) error {
	target, err := s.target(name)
	if err != nil {
		return err
	}
	return os.MkdirAll(target, 0o755)
}

func (s *sink) zip(data []byte) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return rejected("the upload is not a zip: %v", err)
	}
	for _, entry := range reader.File {
		switch {
		case entry.Mode()&os.ModeSymlink != 0:
			return rejected("%q is a link, and a plugin is uploaded with none", entry.Name)
		case entry.FileInfo().IsDir():
			if err := s.folder(entry.Name); err != nil {
				return err
			}
		case entry.Mode().IsRegular() || entry.Mode()&os.ModeType == 0:
			opened, err := entry.Open()
			if err != nil {
				return rejected("%q cannot be read: %v", entry.Name, err)
			}
			err = s.file(entry.Name, entry.Mode(), opened)
			opened.Close()
			if err != nil {
				return err
			}
		default:
			return rejected("%q is not a file or a folder", entry.Name)
		}
	}
	return nil
}

func (s *sink) tarGz(data []byte) error {
	zipped, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return rejected("the upload is not gzip: %v", err)
	}
	defer zipped.Close()
	archive := tar.NewReader(zipped)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return rejected("the upload is not a tar: %v", err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = s.folder(header.Name)
		case tar.TypeReg:
			err = s.file(header.Name, os.FileMode(header.Mode), archive)
		case tar.TypeXGlobalHeader:
		default:
			err = rejected("%q is a %s, and a plugin is uploaded as files and folders only", header.Name, kind(header.Typeflag))
		}
		if err != nil {
			return err
		}
	}
}

func kind(flag byte) string {
	switch flag {
	case tar.TypeSymlink, tar.TypeLink:
		return "link"
	case tar.TypeChar, tar.TypeBlock:
		return "device"
	case tar.TypeFifo:
		return "pipe"
	}
	return fmt.Sprintf("kind of entry (%c) it cannot take", flag)
}
