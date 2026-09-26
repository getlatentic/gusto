package web

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
)

// bundle hands the whole job back as one archive, so a person can check the run
// on a machine this server has no part in.
func (s *Server) bundle(w http.ResponseWriter, r *http.Request) {
	record, dir, found := s.find(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	// A job with no manifest has nothing to check, and the refusal has to happen
	// here: once the first byte of an archive is out the status is spent.
	if _, err := evidence.Read(dir); err != nil {
		http.Error(w, "this job has left no manifest, so there is nothing to verify", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+record.ID+`.tar.gz"`)
	// A failure part way through has nowhere to go but the stream itself. The
	// gzip trailer is never written, so a short bundle refuses to unpack instead
	// of passing for a whole one. Whoever runs the server still needs to know it
	// happened, because the person downloading only sees a broken archive.
	if err := writeBundle(w, dir, record.ID); err != nil {
		slog.Error("a bundle stopped part way through", "job", record.ID, "error", err)
	}
}

// writeBundle writes the job as a gzipped tar under one directory named by the
// job, so unpacking cannot scatter files over whatever directory a person is in.
//
// What goes in is what `haas verify` reads: job.json, whose hash the manifest
// names and whose gate verify re-runs; out/, which it re-hashes; and workspace/,
// which is where the gate runs. Measured on a job whose gate is a script in its
// own checkout: out/ alone stops verify at "no such file or directory" on
// job.json, out/ with job.json re-hashes the seven files and then reports "the
// gate now exits -1, and the manifest says 0" because the gate has no directory
// to run in, and the three together give "7 files match, and the gate still
// exits 0". Carrying job.json gives nothing away that the manifest does not
// already commit to the hash of. state.json stays behind: it is this server's
// cache of what is on disk, and nothing verify does reads it.
//
// tar rather than zip, for the mode bits. The same bundle with gate.sh no longer
// executable reports "the gate now exits -1", so a gate that is a script in the
// checkout is only re-runnable while the bit survives, and tar carries it
// wherever the archive is unpacked. tar is also written in one pass, so the
// archive streams without the size being known before the first byte.
func writeBundle(to io.Writer, dir job.Dir, root string) error {
	deflate := gzip.NewWriter(to)
	archive := tar.NewWriter(deflate)
	if err := addFile(archive, dir.Spec(), path.Join(root, "job.json")); err != nil {
		return err
	}
	for _, branch := range []string{"out", "workspace"} {
		if err := addTree(archive, filepath.Join(dir.Path(), branch), path.Join(root, branch)); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	return deflate.Close()
}

// addTree walks one part of the job. A missing path is not an error: a runner
// that was destroyed takes workspace/ with it and leaves out/ behind, and what
// survives is still worth handing over.
func addTree(archive *tar.Writer, from, to string) error {
	return filepath.WalkDir(from, func(here string, entry fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		within, err := filepath.Rel(from, here)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return add(archive, here, path.Join(to, filepath.ToSlash(within)), info)
	})
}

func addFile(archive *tar.Writer, from, name string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	return add(archive, from, name, info)
}

// add writes one entry. A symlink in a checkout is carried as a link rather than
// followed, so nothing outside the job is ever read into the archive.
func add(archive *tar.Writer, from, name string, info fs.FileInfo) error {
	link := ""
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		link = target
	case !info.Mode().IsRegular() && !info.IsDir():
		return nil // a socket or a device is not evidence
	}
	header, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return err
	}
	header.Name = name
	if info.IsDir() {
		header.Name += "/"
	}
	if err := archive.WriteHeader(header); err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	file, err := os.Open(from)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(archive, file)
	return err
}
