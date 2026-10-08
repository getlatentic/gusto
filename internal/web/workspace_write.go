package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The routes here let a person's computer keep a folder the same as an agent's folder in the cloud. Reading is in
// personal.go. A write is checked twice: that its path stays inside the folder, since a path comes from another machine, and
// that the file is still what the writer saw, since an agent may have changed it in the meantime.

const (
	// mostFileBytes is the largest file the sync moves. A bigger one is left where it is, and the helper says so.
	mostFileBytes = 64 << 20
	// mostHashes is how many paths one request asks the hashes of.
	mostHashes = 5000
	// absent is what a writer says it saw when the file was not there.
	absent = "absent"
)

// errPath is a path that is refused, with why.
var errPath = errors.New("that is not a path inside the folder")

// folderPath is where a path from outside is in the folder, when it is somewhere a sync may put a file: relative, made of
// plain names, not in git's own directory, and not through a link on the way, which could lead out.
func folderPath(root, name string) (string, error) {
	if name == "" || len(name) > 1024 || strings.ContainsAny(name, "\x00\\") || strings.HasPrefix(name, "/") {
		return "", errPath
	}
	parts := strings.Split(name, "/")
	// Every name is checked before any is looked at, so a name that comes after one that does not exist yet is checked too.
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || part == ".git" {
			return "", errPath
		}
	}
	full := root
	for i, part := range parts {
		full = filepath.Join(full, part)
		info, err := os.Lstat(full)
		switch {
		case errors.Is(err, os.ErrNotExist):
			// Nothing below here exists, so nothing below here is a link.
			return filepath.Join(append([]string{root}, parts...)...), nil
		case err != nil:
			return "", err
		case info.Mode()&os.ModeSymlink != 0:
			return "", errPath
		case i < len(parts)-1 && !info.IsDir():
			return "", errPath
		}
	}
	return full, nil
}

// hashes remembers the hash of a file by what it was when it was rememberedHash, so a folder that is asked about again is not read
// again: a file that has the same size and time is the file that was rememberedHash.
type hashes struct {
	mu    sync.Mutex
	known map[string]rememberedHash
}

type rememberedHash struct {
	size, modified int64
	sum            string
}

// mostKnown bounds what is remembered. Past it everything is forgotten and learned again, which is slower and not wrong.
const mostKnown = 200000

func (h *hashes) of(key, path string, info os.FileInfo) (string, error) {
	h.mu.Lock()
	if known, found := h.known[key]; found && known.size == info.Size() && known.modified == info.ModTime().UnixNano() {
		h.mu.Unlock()
		return known.sum, nil
	}
	h.mu.Unlock()
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(digest.Sum(nil))
	h.remember(key, info, sum)
	return sum, nil
}

func (h *hashes) remember(key string, info os.FileInfo, sum string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.known == nil || len(h.known) > mostKnown {
		h.known = map[string]rememberedHash{}
	}
	h.known[key] = rememberedHash{size: info.Size(), modified: info.ModTime().UnixNano(), sum: sum}
}

func (h *hashes) forget(key string) {
	h.mu.Lock()
	delete(h.known, key)
	h.mu.Unlock()
}

// hashesJSON gives the hash of each file asked about, leaving out one that is not there or is not a plain file. The listing
// gives names and sizes cheaply, and this is how a helper learns what a file is only for those that changed.
func (s *Server) hashesJSON(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	var asked struct {
		Paths []string `json:"paths"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&asked) != nil || len(asked.Paths) > mostHashes {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": `send {"paths": [...]}, at most 5000`})
		return
	}
	root := s.instances.Workspace(held.ID)
	out := map[string]string{}
	for _, name := range asked.Paths {
		full, err := folderPath(root, name)
		if err != nil {
			continue
		}
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if sum, err := s.known.of(held.ID+"\x00"+name, full, info); err == nil {
			out[name] = sum
		}
	}
	answerJSON(w, http.StatusOK, map[string]any{"hashes": out})
}

// stillWhatTheWriterSaw says whether the file is as the writer believes: absent, or with the hash they name. A writer that
// names nothing is unconditional.
func (s *Server) stillWhatTheWriterSaw(id, name, full string, saw string) (current string, ok bool, err error) {
	info, statErr := os.Lstat(full)
	switch {
	case errors.Is(statErr, os.ErrNotExist):
		return absent, saw == "" || saw == absent, nil
	case statErr != nil:
		return "", false, statErr
	case !info.Mode().IsRegular():
		return "", false, errPath
	}
	sum, err := s.known.of(id+"\x00"+name, full, info)
	if err != nil {
		return "", false, err
	}
	return sum, saw == "" || saw == sum, nil
}

// writeFile puts a file in the folder whole, or not at all: it is written beside its place and moved into it. It is refused,
// naming the file's present hash, when the writer says the file is something other than what it is.
func (s *Server) writeFile(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	name := r.URL.Query().Get("path")
	root := s.instances.Workspace(held.ID)
	full, err := folderPath(root, name)
	if err != nil {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writing(held.ID).Lock()
	defer s.writing(held.ID).Unlock()
	current, ok, err := s.stillWhatTheWriterSaw(held.ID, name, full, r.Header.Get("X-If-Hash"))
	if err != nil {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !ok {
		answerJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "the file is not what you saw", "hash": current})
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o775); err != nil {
		answerJSON(w, http.StatusInternalServerError, map[string]string{"error": "the folder could not be made"})
		return
	}
	matchOwner(filepath.Dir(full), root)
	scratch, err := os.CreateTemp(filepath.Dir(full), ".haas-write-*")
	if err != nil {
		answerJSON(w, http.StatusInternalServerError, map[string]string{"error": "the file could not be written"})
		return
	}
	defer os.Remove(scratch.Name())
	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(scratch, digest), http.MaxBytesReader(w, r.Body, mostFileBytes))
	if err == nil {
		err = scratch.Sync()
	}
	if closeErr := scratch.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		status := http.StatusInternalServerError
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			status = http.StatusRequestEntityTooLarge
		}
		answerJSON(w, status, map[string]string{"error": "the file could not be written: " + err.Error()})
		return
	}
	mode := os.FileMode(0o664)
	if r.Header.Get("X-Mode") == "755" {
		mode = 0o775
	}
	if err := os.Chmod(scratch.Name(), mode); err != nil {
		answerJSON(w, http.StatusInternalServerError, map[string]string{"error": "the file could not be written"})
		return
	}
	matchOwner(scratch.Name(), root)
	if err := os.Rename(scratch.Name(), full); err != nil {
		answerJSON(w, http.StatusInternalServerError, map[string]string{"error": "the file could not be put in place"})
		return
	}
	if info, err := os.Lstat(full); err == nil {
		s.known.remember(held.ID+"\x00"+name, info, hex.EncodeToString(digest.Sum(nil)))
	}
	answerJSON(w, http.StatusOK, map[string]any{"hash": hex.EncodeToString(digest.Sum(nil)), "size": written})
}

// removeFile deletes a file of the folder, when it is what the writer saw. A file that is not there, and the writer saw not
// there, is as deleted as it gets.
func (s *Server) removeFile(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	name := r.URL.Query().Get("path")
	full, err := folderPath(s.instances.Workspace(held.ID), name)
	if err != nil {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writing(held.ID).Lock()
	defer s.writing(held.ID).Unlock()
	current, ok, err := s.stillWhatTheWriterSaw(held.ID, name, full, r.Header.Get("X-If-Hash"))
	if err != nil {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !ok {
		answerJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "the file is not what you saw", "hash": current})
		return
	}
	if current != absent {
		if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
			answerJSON(w, http.StatusInternalServerError, map[string]string{"error": "the file could not be removed"})
			return
		}
		s.known.forget(held.ID + "\x00" + name)
	}
	w.WriteHeader(http.StatusNoContent)
}

// writing is the lock that makes one writer at a time of an agent's folder, so that checking a file and replacing it is one
// step as far as other writers go. The agent writes without it, and the check is as good as the moment it is made.
func (s *Server) writing(id string) *sync.Mutex {
	s.writers.Lock()
	defer s.writers.Unlock()
	if s.locks == nil {
		s.locks = map[string]*sync.Mutex{}
	}
	if s.locks[id] == nil {
		s.locks[id] = &sync.Mutex{}
	}
	return s.locks[id]
}
