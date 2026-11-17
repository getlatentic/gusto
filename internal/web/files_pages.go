package web

import (
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/internal/instance"
)

// The Files pages' data, in the shapes web/src/pages/files/types.ts names: a place of the person's cloud computer, and one file of
// an agent's folder or of a run's record.

const (
	filesRoute      = "/files"
	fileRoute       = "/instances/{id}/file"
	recordFileRoute = "/jobs/{id}/files/{name}"
)

// pageCrumb is one page above a page, as its bar links to it.
type pageCrumb struct {
	Label string `json:"label"`
	Href  string `json:"href"`
}

// filesHref is the Files page at a place.
func filesHref(at string) string {
	if at == "" {
		return filesRoute
	}
	return filesRoute + "?at=" + url.QueryEscape(at)
}

// backOf is the page the address says the person came from, when it is a page of this server.
func backOf(r *http.Request) string {
	if from := r.URL.Query().Get("from"); ownPath(from) {
		return from
	}
	return ""
}

// placeOf is what a place of the person's cloud computer holds: the top, or a folder of an agent they may see, as the file viewer
// lets them see its files.
func (s *Server) placeOf(r *http.Request) (shownFolders, bool) {
	id, dir := splitAt(r.URL.Query().Get("at"))
	if id == "" {
		return s.topFolders(r), true
	}
	held, found := s.instances.Get(id)
	if !found || !s.inTeam(r, held.Team) || !(s.mayUse(r, held.Who) || s.inRoom(r, held)) {
		return shownFolders{}, false
	}
	here, ok := s.agentFolderDir(id, dir)
	if !ok {
		return shownFolders{}, false
	}
	return s.insideFolders(held, dir, here), true
}

func (s *Server) hasPlace(r *http.Request) bool {
	_, found := s.placeOf(r)
	return found
}

// filesData is the Files page: the place it is open at, and the page the person came from.
func (s *Server) filesData(r *http.Request) (map[string]any, error) {
	here, found := s.placeOf(r)
	if !found {
		return nil, failed{http.StatusNotFound, "there is no such folder"}
	}
	return map[string]any{"Here": here, "Back": backOf(r)}, nil
}

// folderPage is one agent's folder, which is the Files page open at it.
func (s *Server) folderPage(w http.ResponseWriter, r *http.Request) {
	held, found := s.sharedOr404(w, r)
	if !found {
		return
	}
	to := filesHref(held.ID)
	if back := backOf(r); back != "" {
		to += "&from=" + url.QueryEscape(back)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// foundFile is a file of an agent's folder that the address names, resolved inside the folder.
type foundFile struct {
	held     instance.Instance
	path     string
	resolved string
	info     os.FileInfo
}

// fileAsked is the file the address names. The path is resolved inside the folder and refused if it leaves it, a link included, so
// nothing else on the machine can be asked for.
func (s *Server) fileAsked(r *http.Request) (foundFile, bool) {
	held, found := s.shared(r)
	if !found {
		return foundFile{}, false
	}
	asked := filepath.Clean("/" + r.URL.Query().Get("path"))
	resolved, ok := resolveInFolder(s.instances.AgentFolder(held.ID), asked)
	if !ok {
		return foundFile{}, false
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return foundFile{}, false
	}
	return foundFile{held: held, path: strings.TrimPrefix(asked, "/"), resolved: resolved, info: info}, true
}

func (s *Server) hasFile(r *http.Request) bool {
	_, found := s.fileAsked(r)
	return found
}

// fileData is one file of an agent's folder as it is now, in the way its kind is read: text and Markdown as words, a picture as a
// picture, a PDF in the page, a web page running in a sealed frame, a document, slide deck or sheet as the words it says, a table
// as a table. Prose is a Markdown file drawn, Save where a change to it is sent, and Source the address of a web page's source.
func (s *Server) fileData(r *http.Request) (map[string]any, error) {
	found, ok := s.fileAsked(r)
	if !ok {
		return nil, failed{http.StatusNotFound, "there is no such file"}
	}
	file, err := s.viewedIn(found, r.URL.Query().Get("source") != "")
	if err != nil {
		return nil, err
	}
	id := found.held.ID
	here := "/instances/" + id + "/file?path=" + url.QueryEscape(file.Path)
	page := map[string]any{"File": file, "Back": backOf(r), "BackWord": "", "Crumbs": s.fileCrumbs(found.held, file.Path),
		"Prose": "", "Save": "", "Source": ""}
	if file.Kind == kindMarkdown {
		page["Prose"] = string(s.replyIn(file.Text, id, file.Dir, here))
	}
	if file.Editable {
		page["Save"] = here
	}
	if file.Kind == kindHTML {
		page["Source"] = here + "&source=1"
	}
	return page, nil
}

// viewedIn reads a found file for the viewer. A web page asked for as its source is shown as text.
func (s *Server) viewedIn(found foundFile, source bool) (viewedFile, error) {
	head, err := readSome(found.resolved, 8<<10)
	if err != nil && found.info.Size() > 0 {
		return viewedFile{}, failed{http.StatusNotFound, "the file could not be read"}
	}
	name := path.Base(found.path)
	file := viewedFile{Name: name, Path: found.path, Dir: strings.TrimPrefix(path.Dir("/"+found.path), "/"),
		Kind: kindOf(name, head), Size: measure(found.info.Size()), Modified: found.info.ModTime().Unix()}
	file.Raw = "/instances/" + found.held.ID + "/raw?path=" + url.QueryEscape(file.Path)
	file.Download = file.Raw + "&download=1"
	if file.Kind == kindHTML && source {
		file.Kind = kindText
	}
	s.fill(&file, found.resolved, found.info.Size())
	editable(&file, found.resolved, found.info.Size(), found.held.Archived != 0)
	return file, nil
}

// fileCrumbs is where a file is: Files, the agent, then each folder it is in.
func (s *Server) fileCrumbs(held instance.Instance, file string) []pageCrumb {
	crumbs := []pageCrumb{{Label: "Files", Href: filesRoute}, {Label: s.agentOf(held).Name, Href: filesHref(held.ID)}}
	walked := held.ID
	parts := strings.Split(file, "/")
	for _, part := range parts[:len(parts)-1] {
		walked += "/" + part
		crumbs = append(crumbs, pageCrumb{Label: part, Href: filesHref(walked)})
	}
	return crumbs
}
