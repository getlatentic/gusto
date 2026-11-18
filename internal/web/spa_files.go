package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"sync"
)

// The React app in web/ is built into spa/dist by web/build.sh and carried in the binary, so each server is still one artifact.
// The built files are not committed: spa/README.md is, so a checkout that has not built the app still compiles.
//
//go:embed all:spa
var spaFiles embed.FS

// An entry is one of the app's built pages. The product's pages are one; gusto cloud's own pages, its landing page among them, are
// another, so a visitor's first page does not load the product's code.
type Entry string

const (
	ProductEntry Entry = "index.html"
	CloudEntry   Entry = "cloud.html"
)

// builtPages is each entry's page as built, read once.
var builtPages = map[Entry]func() ([]byte, error){
	ProductEntry: readOnce(ProductEntry),
	CloudEntry:   readOnce(CloudEntry),
}

func readOnce(entry Entry) func() ([]byte, error) {
	return sync.OnceValues(func() ([]byte, error) { return spaFiles.ReadFile("spa/dist/" + string(entry)) })
}

// WriteAppPage answers with an entry's page: the server's own head in place of <!--gusto-head-->, and the page's data, already
// JSON, in place of <!--gusto-data--> where it is given. JSON from encoding/json writes < > & as escapes, so the data cannot end
// the script that holds it. A page under a path, base, loads its build and its static files from under that path (underBase).
func WriteAppPage(w http.ResponseWriter, status int, entry Entry, base, head, data string) {
	page, err := builtPages[entry]()
	if err != nil {
		http.Error(w, "this gusto was built without its web app: run web/build.sh, then build gusto again", http.StatusInternalServerError)
		return
	}
	page = bytes.Replace(page, []byte("<!--gusto-head-->"), []byte(head), 1)
	page = underBase(page, base)
	if data != "" {
		page = bytes.Replace(page, []byte("<!--gusto-data-->"), []byte(`<script id="gusto-data" type="application/json">`+data+`</script>`), 1)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(page)
}

// underBase puts a path in front of every link a page makes to the app's built files and to /static. A server reached under a
// path, as gusto cloud serves an organization at /workspace/<slug>, then answers for its own page's files, so a page is never
// given another release's: in a deploy gusto cloud's own build is newer than an organization's server until that restarts. A
// built file finds the files it loads beside itself (web/vite.config.ts, renderBuiltUrl), so the links in the page are all
// that need the path.
func underBase(page []byte, base string) []byte {
	if base == "" {
		return page
	}
	for _, root := range []string{"/app/assets/", "/static/"} {
		page = bytes.ReplaceAll(page, []byte(`="`+root), []byte(`="`+base+root))
	}
	return page
}

// SharedHead is what every page of the app loads from /static before its own files, each named with its fingerprint: the icons,
// the tokens, the faces and the theme, and the script that sets the person's theme before anything is drawn.
func SharedHead() string {
	return `<link rel="icon" type="image/svg+xml" href="` + asset("/static/mark.svg") + `">` +
		`<link rel="icon" type="image/png" sizes="32x32" href="` + asset("/static/favicon-32.png") + `">` +
		`<link rel="apple-touch-icon" href="` + asset("/static/apple-touch-icon.png") + `">` +
		`<link rel="stylesheet" href="` + asset("/static/tokens.css") + `">` +
		`<link rel="stylesheet" href="` + asset("/static/fonts.css") + `">` +
		`<link rel="stylesheet" href="` + asset("/static/theme.css") + `">` +
		`<script src="` + asset("/static/look.js") + `"></script>`
}

// AppAssets serves the app's built files under /app/assets/. A built file's name carries a hash of what is in it, so a browser
// keeps it until a new build names another.
func AppAssets() http.Handler {
	assets, err := fs.Sub(spaFiles, "spa/dist/assets")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/app/assets/", http.FileServerFS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		files.ServeHTTP(w, r)
	})
}
