package vmwall

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Download writes what is at url to dst, within ctx.
type Download func(ctx context.Context, url string, dst io.Writer) error

// OverHTTP is the Download that asks the server named in the address.
func OverHTTP(ctx context.Context, url string, dst io.Writer) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", url, answer.Status)
	}
	_, err = io.Copy(dst, answer.Body)
	return err
}

// fetch downloads url into a file in dir and returns its path, once its SHA-256 is the one expected. A file that does not match is
// removed: nothing unchecked is left where it could be run.
func fetch(ctx context.Context, get Download, url, want, dir string) (string, error) {
	file, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	sum := sha256.New()
	err = get(ctx, url, io.MultiWriter(file, sum))
	if closed := file.Close(); err == nil {
		err = closed
	}
	if err == nil && hex.EncodeToString(sum.Sum(nil)) != want {
		err = fmt.Errorf("%s is not the file haas expected: its checksum differs", url)
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// unpackOne writes the one file of a .tar.gz archive to dst, executable. An archive with more than one file, or a link, is refused:
// an archive here holds a program and nothing else.
func unpackOne(archive, dst string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	unzipped, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	reader := tar.NewReader(unzipped)
	var found bool
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg || found {
			return fmt.Errorf("%s holds more than one program", filepath.Base(archive))
		}
		found = true
		if err := writeProgram(dst, reader); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("%s holds no program", filepath.Base(archive))
	}
	return nil
}

// writeProgram makes dst whole or not at all, so a program that is running is never replaced by half of one.
func writeProgram(dst string, from io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	staged, err := os.CreateTemp(filepath.Dir(dst), ".program-*")
	if err != nil {
		return err
	}
	defer os.Remove(staged.Name())
	if _, err := io.Copy(staged, from); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Chmod(0o755); err != nil {
		staged.Close()
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	return os.Rename(staged.Name(), dst)
}

// unpackInto writes each file of a .tar.gz archive to the path where names it, or skips the ones where returns empty. A link or a
// path with a folder in it is refused: the archive holds files and nothing else.
func unpackInto(archive string, where func(name string) string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	unzipped, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	reader := tar.NewReader(unzipped)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		name := filepath.Base(header.Name)
		if header.Typeflag != tar.TypeReg || name != header.Name {
			return fmt.Errorf("%s holds %q, which is not a plain file", filepath.Base(archive), header.Name)
		}
		if dst := where(name); dst != "" {
			if err := writeProgram(dst, reader); err != nil {
				return err
			}
		}
	}
}
