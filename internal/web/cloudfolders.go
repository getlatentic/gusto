package web

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/job"
)

// A folder of the person's cloud computer is named by where it is: nothing for the top, where their agents' folders are, then an agent's
// id, then a path from the root of that agent's workspace ("20261009-0211-alpha/src/app"). Nothing outside a workspace can be named.

// shownFolder is one folder in the explorer.
type shownFolder struct {
	Name string `json:"name"`
	At   string `json:"at"`
	// Harness and Model say what works in it, for a folder that is an agent's whole workspace.
	Harness string `json:"harness,omitempty"`
}

type crumb struct {
	Name string `json:"name"`
	At   string `json:"at"`
}

// shownFolders is what the explorer shows of one place: where it is, the folders in it, and the files.
type shownFolders struct {
	At      string        `json:"at"`
	Crumbs  []crumb       `json:"crumbs"`
	Folders []shownFolder `json:"folders"`
	Files   []string      `json:"files"`
	// Harness is what works in this place, when it is inside an agent's folder.
	Harness string `json:"harness,omitempty"`
}

const filesShown = 60

// splitAt separates "id/some/path" into the agent and the path in its workspace.
func splitAt(at string) (id, dir string) {
	id, dir, _ = strings.Cut(strings.Trim(at, "/"), "/")
	return id, dir
}

// folderAgent is the agent a place names, when it is one of the person's.
func (s *Server) folderAgent(r *http.Request, id string) (instance.Instance, bool) {
	held, found := s.instances.Get(id)
	if !found || !s.mayUse(r, held.Who) {
		return instance.Instance{}, false
	}
	return held, true
}

// workspaceDir is the folder of an agent's workspace that a path names, resolved so that a link inside the workspace cannot lead out of it.
func (s *Server) workspaceDir(id, dir string) (string, bool) {
	cleaned, err := job.CleanDir(dir)
	if err != nil {
		return "", false
	}
	root, err := filepath.EvalSymlinks(s.instances.Workspace(id))
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(cleaned)))
	if err != nil || (resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator))) {
		return "", false
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return "", false
	}
	return resolved, true
}

// chatFolders answers the explorer: what is in a place of the person's cloud computer.
func (s *Server) chatFolders(w http.ResponseWriter, r *http.Request) {
	id, dir := splitAt(r.URL.Query().Get("at"))
	if id == "" {
		answerJSON(w, http.StatusOK, s.topFolders(r))
		return
	}
	held, found := s.folderAgent(r, id)
	if !found {
		http.NotFound(w, r)
		return
	}
	here, ok := s.workspaceDir(id, dir)
	if !ok {
		http.NotFound(w, r)
		return
	}
	answerJSON(w, http.StatusOK, s.insideFolders(held, dir, here))
}

// topFolders is the person's agents, each the top of a folder of their own.
func (s *Server) topFolders(r *http.Request) shownFolders {
	shown := shownFolders{Crumbs: []crumb{{Name: "Your cloud computer"}}, Folders: []shownFolder{}, Files: []string{}}
	for _, each := range s.lastUsed(r) {
		shown.Folders = append(shown.Folders, shownFolder{Name: each.Name, At: each.ID, Harness: each.Harness})
	}
	return shown
}

func (s *Server) insideFolders(held instance.Instance, dir, here string) shownFolders {
	name := s.agentOf(held).Name
	shown := shownFolders{At: path.Join(held.ID, dir), Harness: held.Harness, Folders: []shownFolder{}, Files: []string{},
		Crumbs: []crumb{{Name: "Your cloud computer"}, {Name: name, At: held.ID}}}
	walked := ""
	for _, part := range strings.Split(dir, "/") {
		if part == "" {
			continue
		}
		walked += "/" + part
		shown.Crumbs = append(shown.Crumbs, crumb{Name: part, At: held.ID + walked})
	}
	entries, _ := os.ReadDir(here)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		switch {
		case entry.IsDir():
			shown.Folders = append(shown.Folders, shownFolder{Name: entry.Name(), At: path.Join(held.ID, dir, entry.Name())})
		case entry.Type().IsRegular() && len(shown.Files) < filesShown:
			shown.Files = append(shown.Files, entry.Name())
		}
	}
	sort.Slice(shown.Folders, func(a, b int) bool { return shown.Folders[a].Name < shown.Folders[b].Name })
	sort.Strings(shown.Files)
	return shown
}

// plainFolderName is a name for one folder: a single part, which is not hidden and holds nothing a path would.
func plainFolderName(name string) bool {
	if name == "" || len(name) > 80 || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return false
	}
	for _, c := range name {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}

// newChatFolder makes a folder: inside an agent's folder it is a folder of its workspace, and at the top it is a new agent's own.
func (s *Server) newChatFolder(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if !plainFolderName(name) {
		http.Error(w, "a folder is named with one plain word or phrase", http.StatusBadRequest)
		return
	}
	id, dir := splitAt(r.FormValue("at"))
	if id == "" {
		s.newTopFolder(w, r, name)
		return
	}
	if _, found := s.folderAgent(r, id); !found {
		http.NotFound(w, r)
		return
	}
	parent, ok := s.workspaceDir(id, dir)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := os.Mkdir(filepath.Join(parent, name), 0o755); err != nil && !os.IsExist(err) {
		http.Error(w, "that folder could not be made", http.StatusInternalServerError)
		return
	}
	answerJSON(w, http.StatusOK, map[string]string{"at": path.Join(id, dir, name)})
}

func (s *Server) newTopFolder(w http.ResponseWriter, r *http.Request, name string) {
	started, _, why := s.beginInstance(r, instance.Instance{Harness: strings.TrimSpace(r.FormValue("harness")), Title: name, Who: s.keeper.who(r)})
	if why != "" {
		http.Error(w, why, http.StatusBadRequest)
		return
	}
	answerJSON(w, http.StatusOK, map[string]string{"at": started.ID})
}

// cloudFolderThread is the agent and the conversation a chat in a folder of the cloud computer goes to, on the model chosen.
func (s *Server) cloudFolderThread(r *http.Request, at, model string) (instance.Instance, instance.Thread, string) {
	id, dir := splitAt(at)
	held, found := s.folderAgent(r, id)
	if !found {
		return held, instance.Thread{}, "that folder is not one of yours"
	}
	if _, ok := s.workspaceDir(id, dir); !ok {
		return held, instance.Thread{}, "that folder is not there"
	}
	dir, _ = job.CleanDir(dir)
	if why := s.unusable(r, held.Harness, model); why != "" {
		return held, instance.Thread{}, why
	}
	if model != "" && model != held.Model {
		if err := s.instances.SetModel(held.ID, model); err != nil {
			return held, instance.Thread{}, err.Error()
		}
		held.Model = model
	}
	if thread := s.instances.Latest(held.ID); thread.Dir == dir && len(thread.Turns) == 0 {
		return held, thread, ""
	}
	thread, err := s.instances.NewThreadIn(held.ID, "", dir)
	if err != nil {
		return held, thread, err.Error()
	}
	return held, thread, ""
}
