package web

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getlatentic/haas/internal/job"
)

// changed is the files a turn changed, from its patch, for the chat to link to.
func changed(dir job.Dir) []string {
	if !dir.PlainOut("diff.patch") {
		return nil
	}
	patch, err := os.ReadFile(dir.OutFile("diff.patch"))
	if err != nil {
		return nil
	}
	var names []string
	for _, file := range parseDiff(string(patch)) {
		names = append(names, file.Name)
	}
	return names
}

// mostShown is how much of a file the viewer draws. A folder can hold
// anything, and a page is not the place to read a gigabyte.
const mostShown = 512 << 10

// folderFile shows one file from a harness's folder as it is now. The path is
// resolved inside the folder and refused if it leaves it, a link included, so
// nothing else on the machine can be asked for.
func (s *Server) folderFile(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	root, err := filepath.EvalSymlinks(s.instances.Workspace(held.ID))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	asked := filepath.Clean("/" + r.URL.Query().Get("path"))
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, asked))
	if err != nil || !strings.HasPrefix(resolved, root+string(filepath.Separator)) || strings.Contains(asked, "/.git/") || strings.HasPrefix(asked, "/.git") {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	text, err := readSome(resolved)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "file.html", map[string]any{
		"Title":    "haas — " + filepath.Base(asked),
		"Nav":      "harnesses",
		"Instance": held,
		"Agent":    s.agentOf(held),
		"Path":     strings.TrimPrefix(asked, "/"),
		"Text":     string(text),
		"Cut":      info.Size() > mostShown,
		"Markdown": strings.EqualFold(filepath.Ext(asked), ".md"),
		"Back":     r.URL.Query().Get("from"),
	})
}

func readSome(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	text := make([]byte, mostShown)
	read, err := file.Read(text)
	if err != nil && read == 0 {
		return nil, fs.ErrNotExist
	}
	return text[:read], nil
}

// held is one file in an agent's folder, as its Files page lists it.
type held struct {
	Path     string
	Size     int64
	Modified int64
}

// mostListed is how many files the Files page lists. A folder a build ran in
// can hold hundreds of thousands, and nobody reads that list.
const mostListed = 500

// folderPage lists what is in an agent's folder, the most recently changed
// first, because that is what a conversation has been working on.
func (s *Server) folderPage(w http.ResponseWriter, r *http.Request) {
	agentHeld, found := s.mine(w, r)
	if !found {
		return
	}
	root := s.instances.Workspace(agentHeld.ID)
	var files []held
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !entry.Type().IsRegular() || len(files) >= mostListed {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		files = append(files, held{Path: filepath.ToSlash(relative), Size: info.Size(), Modified: info.ModTime().Unix()})
		return nil
	})
	sort.Slice(files, func(a, b int) bool { return files[a].Modified > files[b].Modified })
	s.render(w, "files.html", map[string]any{
		"Title": "haas — files",
		"Nav":   "harnesses",
		"Agent": s.agentOf(agentHeld),
		"Files": files,
		"Cut":   len(files) >= mostListed,
		"Back":  r.URL.Query().Get("from"),
	})
}
