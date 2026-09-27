package web

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/job"
)

// changed is the files a turn changed, from its patch, for the chat to link to.
func changed(dir job.Dir) []string {
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
	held, found := s.instances.Get(r.PathValue("id"))
	if !found {
		http.NotFound(w, r)
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
