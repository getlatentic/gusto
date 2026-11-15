package web

import (
	"bytes"
	"cmp"
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"sync"
)

// The React app in web/ is built into spa/dist and carried in the binary, so a page that has moved to it is still one artifact.
// Its pages are served at their own addresses, behind the same sign-in as the pages drawn here, and read their data from
// /app/api/, which answers the same views the templates were given, as JSON.
//
//go:embed all:spa/dist
var spaFiles embed.FS

// failed is why a page's data could not be given, with the status that says so.
type failed struct {
	status int
	why    string
}

func (f failed) Error() string { return f.why }

// answerFailure answers a request whose data could not be gathered.
func answerFailure(w http.ResponseWriter, r *http.Request, err error) {
	var reason failed
	switch {
	case errors.As(err, &reason) && reason.status == http.StatusNotFound:
		http.NotFound(w, r)
	case errors.As(err, &reason):
		http.Error(w, reason.why, reason.status)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// appView is one console page the React app draws: its address, whether this server has it to show, and the data it reads.
type appView struct {
	path   string
	has    func() bool
	gather func(*http.Request) (map[string]any, error)
}

func always() bool { return true }

// appViews are the console's pages that have moved to the React app.
func (s *Server) appViews() []appView {
	return []appView{
		{"/admin", always, s.overviewData},
		{"/admin/spending", always, func(r *http.Request) (map[string]any, error) { return s.spendData(r, "/admin/spending") }},
		{"/admin/usage", always, func(r *http.Request) (map[string]any, error) { return s.usageData(r, "/admin/usage", true) }},
		{"/admin/usage/{session}", always, func(r *http.Request) (map[string]any, error) { return s.sessionData(r, "/admin/usage") }},
		{peoplePath, s.hasRoster, s.peopleData},
		{budgetsPath, s.hasOrgSettings, s.budgetsData},
		{tokensPath, s.hasTokens, s.tokensData},
		{recordsPath, always, s.recordsData},
		{modelsPath, s.hasRegistry, s.modelsData},
		{appsPath, s.hasAppsOrSkills, s.appsData},
		{setupsPath + "/{name}", s.hasSetups, s.skillSetData},
		{integrationsPath + "/{name}", s.hasIntegrations, s.integrationData},
	}
}

// mountApp serves each of the app's pages at its own address and its data under /app/api, behind the console's sign-in.
func (s *Server) mountApp(mux *http.ServeMux) {
	for _, view := range s.appViews() {
		mux.HandleFunc("GET "+view.path, s.console(offering(view.has, s.appPage)))
		mux.HandleFunc("GET /app/api"+view.path, s.console(offering(view.has, s.appData(view.gather))))
	}
	mux.HandleFunc("GET /app/api/console", s.console(s.consoleJSON))
	assets, err := fs.Sub(spaFiles, "spa/dist/assets")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/app/assets/", http.FileServerFS(assets))
	// A built file's name carries a hash of what is in it, so a browser keeps it until a new build names another.
	mux.HandleFunc("GET /app/assets/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		files.ServeHTTP(w, r)
	})
}

// appHead is what the app's page loads from /static before its own files: the tokens, the faces and the theme, and the script
// that sets the person's theme before anything is drawn. Each is named with its fingerprint, as the templates name them.
func appHead() string {
	return `<link rel="icon" type="image/svg+xml" href="` + asset("/static/mark.svg") + `">` +
		`<link rel="stylesheet" href="` + asset("/static/tokens.css") + `">` +
		`<link rel="stylesheet" href="` + asset("/static/fonts.css") + `">` +
		`<link rel="stylesheet" href="` + asset("/static/theme.css") + `">` +
		`<script src="` + asset("/static/look.js") + `"></script>`
}

// appIndex is the app's one page as built, with its head filled in, made once.
var appIndex = sync.OnceValues(func() ([]byte, error) {
	page, err := spaFiles.ReadFile("spa/dist/index.html")
	if err != nil {
		return nil, err
	}
	return bytes.Replace(page, []byte("<!--gusto-head-->"), []byte(appHead()), 1), nil
})

// appPage is the React app's one page, which draws whichever of its pages the address names.
func (s *Server) appPage(w http.ResponseWriter, _ *http.Request) {
	page, err := appIndex()
	if err != nil {
		http.Error(w, "the app was not built into this server", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(page)
}

// appData answers a page's data as JSON.
func (s *Server) appData(gather func(*http.Request) (map[string]any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := gather(r)
		if err != nil {
			answerFailure(w, r, err)
			return
		}
		delete(page, "Title")
		// A page's data is read fresh each time: it can hold what only one answer may show, as a new token's secret.
		w.Header().Set("Cache-Control", "no-store")
		s.asJSON(w, page)
	}
}

// consoleShell is what every console page draws around itself: its sections, the organization it is for, and the way back.
type consoleShell struct {
	Sections []consoleSection `json:"sections"`
	OrgName  string           `json:"orgName,omitempty"`
	OrgPage  string           `json:"orgPage,omitempty"`
}

type consoleSection struct {
	Key   string `json:"key"`
	Href  string `json:"href"`
	Label string `json:"label"`
}

func (s *Server) consoleJSON(w http.ResponseWriter, _ *http.Request) {
	shell := consoleShell{}
	for _, each := range s.sections() {
		shell.Sections = append(shell.Sections, consoleSection{Key: each.Key, Href: each.Href, Label: each.Label})
	}
	if s.front != nil && s.front.Org != "" {
		shell.OrgName, shell.OrgPage = cmp.Or(s.front.Name, s.front.Org), s.front.page("/org", nil)
	}
	s.asJSON(w, shell)
}
