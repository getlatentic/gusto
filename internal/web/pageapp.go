package web

import (
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/internal/apps"
)

// servePage answers for a page an agent built: the files of its folder, as they are now, and index.html for a folder.
// At an origin of its own the page keeps what it saves in the browser, as it would anywhere else; under a path of this
// server's it runs sandboxed, like any app served there.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request, app apps.App, prefix string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "a page is only read", http.StatusMethodNotAllowed)
		return
	}
	asked := path.Clean("/" + strings.TrimPrefix(r.URL.Path, prefix))
	folder := filepath.Join(s.instances.Workspace(app.Instance), filepath.FromSlash(app.Folder))
	resolved, found := pageFile(folder, asked)
	if !found {
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
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", pageType(resolved))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	if prefix != "" {
		h.Set("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups allow-modals")
	}
	http.ServeContent(w, r, resolved, info.ModTime(), file)
}

// pageFile is the file a page's path names in its folder, and the folder's index.html for a folder. Nothing outside the
// folder is found, through a link or otherwise.
func pageFile(folder, asked string) (string, bool) {
	if asked == "/" {
		asked = "/index.html"
	}
	resolved, ok := resolveInFolder(folder, asked)
	if !ok {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return pageFile(folder, path.Join(asked, "index.html"))
	}
	return resolved, info.Mode().IsRegular()
}

// pageType is what a page's file is served as: what its name says, since the page is its own site and its scripts and
// styles are meant to run.
func pageType(name string) string {
	if known := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); known != "" {
		return known
	}
	return "application/octet-stream"
}
