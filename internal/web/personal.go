package web

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getlatentic/haas/internal/apitoken"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/quota"
)

// The routes below are a person's own work as JSON, for the helper on their computer, an editor and a tray. They are
// reached with the token the device flow gives, which acts as the person, and each answers about that person's own
// objects: somebody else's is answered as a missing one, as the pages do.

// asPerson guards a route a person's token reaches. A shared password, or any token that does not act as a person, is
// refused, since these routes are about somebody.
func (s *Server) asPerson(next http.HandlerFunc) http.HandlerFunc {
	return s.sent(apitoken.Work, func(w http.ResponseWriter, r *http.Request) {
		if personOf(r) == "" {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	})
}

// threadWithTurns is a conversation and what was said in it, each turn the job it was.
type threadWithTurns struct {
	instance.Thread
	Said []jobs.Record `json:"said"`
}

func (s *Server) instancesJSON(w http.ResponseWriter, r *http.Request) {
	own := s.ownInstances(r, s.instances.List())
	if own == nil {
		own = []instance.Instance{}
	}
	answerJSON(w, http.StatusOK, own)
}

func (s *Server) instanceJSON(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	answerJSON(w, http.StatusOK, map[string]any{"instance": held, "threads": s.threadsOf(held.ID)})
}

func (s *Server) threadsJSON(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	answerJSON(w, http.StatusOK, s.threadsOf(held.ID))
}

// threadsOf is an instance's conversations, an empty list and not null when it has none.
func (s *Server) threadsOf(id string) []instance.Thread {
	if threads := s.instances.Threads(id); threads != nil {
		return threads
	}
	return []instance.Thread{}
}

func (s *Server) threadJSON(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	thread, found := s.instances.Thread(held.ID, r.PathValue("thread"))
	if !found {
		http.NotFound(w, r)
		return
	}
	said := []jobs.Record{}
	for _, id := range thread.Turns {
		if record, ok := s.index.Get(id); ok {
			said = append(said, record)
		}
	}
	answerJSON(w, http.StatusOK, threadWithTurns{Thread: thread, Said: said})
}

// workspaceFile is one file of an agent's folder as a helper needs it to keep a copy.
type workspaceFile struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

// mostFiles bounds one listing. A folder a build ran in can hold hundreds of thousands, and a copy of one is not the
// helper's job: it says the list was cut, and ignore rules are what keep it short.
const mostFiles = 50000

// filesJSON lists the regular files of an agent's folder. A link is not followed or listed, and git's own directory is not
// part of the work.
func (s *Server) filesJSON(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	root := s.instances.Workspace(held.ID)
	files, cut := []workspaceFile{}, false
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case entry.IsDir() && entry.Name() == ".git":
			return fs.SkipDir
		case !entry.Type().IsRegular():
			return nil
		case len(files) >= mostFiles:
			cut = true
			return fs.SkipAll
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, workspaceFile{Path: filepath.ToSlash(relative), Size: info.Size(), Modified: info.ModTime().Unix()})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	answerJSON(w, http.StatusOK, map[string]any{"files": files, "cut": cut})
}

// fileBytes gives one file of an agent's folder whole, with ranges, resolved inside the folder as the viewer does.
func (s *Server) fileBytes(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	asked := filepath.Clean("/" + r.URL.Query().Get("path"))
	resolved, ok := resolveInFolder(s.instances.Workspace(held.ID), asked)
	if !ok {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(resolved)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, strings.TrimPrefix(asked, "/"), info.ModTime(), file)
}

// newThreadJSON opens a conversation on one of the person's agents.
func (s *Server) newThreadJSON(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	var asked struct {
		Title string `json:"title"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&asked)
	thread, err := s.instances.NewThread(held.ID, strings.TrimSpace(asked.Title))
	if err != nil {
		answerJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	answerJSON(w, http.StatusCreated, thread)
}

// saidJSON is one turn said in a person's own thread. An Idempotency-Key makes a retry the same turn: the job the first
// send made is given back, with `already` true, and nothing is started.
func (s *Server) saidJSON(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	thread, found := s.instances.Thread(held.ID, r.PathValue("thread"))
	if !found {
		http.NotFound(w, r)
		return
	}
	key, err := requestID(r)
	if err != nil {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var said struct {
		Said string `json:"said"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&said) != nil || strings.TrimSpace(said.Said) == "" {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": `send {"said": "..."}: what to say in this thread`})
		return
	}
	record, already, err := s.startTurn(personOf(r), held, thread, strings.TrimSpace(said.Said), key)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, quota.ErrExceeded) {
			status = http.StatusTooManyRequests
		}
		answerJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	answerJSON(w, http.StatusAccepted, map[string]any{"job": record.ID, "state": record.State, "already": already})
}
