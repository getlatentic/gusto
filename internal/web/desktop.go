package web

import (
	"net/http"
	"os"
	"strings"
)

// UseDesktop gives the server a folder of the desktop app's releases to hand out: a latest.json that says what the newest version is and
// where each system's update is, and the updates it names. The app updates from the server it is connected to, so an organisation
// that runs its own server decides when its people update, and a server with no folder hands out nothing. Every file in it is signed
// with a key the app holds the public half of, so the server is trusted to hand files out and not to vouch for them.
func (s *Server) UseDesktop(dir string) { s.desktop = dir }

func (s *Server) desktopRoutes(mux *http.ServeMux) {
	if s.desktop == "" {
		return
	}
	files := http.StripPrefix("/desktop/", http.FileServer(noListing{http.Dir(s.desktop)}))
	mux.HandleFunc("GET /desktop/", func(w http.ResponseWriter, r *http.Request) {
		// A person's app asks for this with no sign-in, as it asks before it has signed in to anything.
		if strings.HasSuffix(r.URL.Path, ".json") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// noListing is a folder that shows its files to a request for one of them and shows nothing of itself.
type noListing struct{ root http.FileSystem }

func (n noListing) Open(name string) (http.File, error) {
	file, err := n.root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if info.IsDir() {
		file.Close()
		return nil, os.ErrNotExist
	}
	return file, nil
}
