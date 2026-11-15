package web

import (
	"cmp"
	"errors"
	"html"
	"net/http"
)

// appDataPrefix is where every page of the app reads its data.
const appDataPrefix = "/app/api/"

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

// conversationRoute is a conversation's page.
const conversationRoute = "/instances/{id}/threads/{thread}"

// personViews are a person's own pages that have moved to the React app.
func (s *Server) personViews() []appView {
	return []appView{
		{"/chat", always, s.chatData},
		{"/rooms/new", always, s.newRoomData},
		{conversationRoute, always, s.conversationData},
		{"/members", always, s.membersData},
		{agentRoute, always, s.agentData},
		{personRoute, always, s.personData},
		// The work pages: Activity, a project, how each agent type did, and comparing agents. A task's page is served by
		// workRoutes, as it sends a turn to its conversation.
		{jobsRoute, always, s.jobsData},
		{projectRoute, always, s.projectData},
		{"/engines", always, s.enginesData},
		{"/compare", always, s.compareData},
		{lineupRoute, always, s.lineupData},
		{filesRoute, always, s.filesData},
		{fileRoute, always, s.fileData},
		{"/setups", always, s.setupsData},
		{"/setups/{name}", s.hasSetups, s.setupData},
		{"/presets", always, s.presetsData},
		{"/templates/{name}", always, s.templateData},
		{"/new/blank", always, s.blankData},
		{"/usage", always, func(r *http.Request) (map[string]any, error) { return s.usageData(r, "/usage", false) }},
		{"/usage/{session}", always, func(r *http.Request) (map[string]any, error) { return s.sessionData(r, "/usage") }},
		{"/spend", always, func(r *http.Request) (map[string]any, error) { return s.spendData(r, "/spend") }},
		{connectionsPath, s.hasConnectors, s.connectionsData},
		{devicePath, s.hasTokens, s.deviceData},
	}
}

// publicViews are the app's pages that are served before anybody is signed in. Each page's own handler decides whether to draw
// it (see publicPage), and its data is open to whoever asks.
func (s *Server) publicViews() []appView {
	return []appView{
		{"/login", always, s.loginData},
	}
}

// pageFound is, for a page whose address names something that may not be there, whether it is, so such a page answers 404 and
// not the app. It is nil for a page that is always there.
func (s *Server) pageFound(route string) func(*http.Request) bool {
	return map[string]func(*http.Request) bool{
		conversationRoute:   s.hasConversation,
		agentRoute:          s.hasAgent,
		personRoute:         s.hasPerson,
		filesRoute:          s.hasPlace,
		fileRoute:           s.hasFile,
		"/setups/{name}":    s.hasSetup,
		"/templates/{name}": s.hasTemplate,
		projectRoute:        s.hasProject,
		lineupRoute:         s.hasLineup,
	}[route]
}

// whenThere serves next when the address names something that is there, and answers 404 when it does not.
func whenThere(there func(*http.Request) bool, next http.HandlerFunc) http.HandlerFunc {
	if there == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !there(r) {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

// mountApp serves each of the app's pages at its own address and its data under /app/api: the console's behind the console's
// sign-in, and a person's own behind theirs.
func (s *Server) mountApp(mux *http.ServeMux) {
	mount := func(views []appView, admit func(http.HandlerFunc) http.HandlerFunc) {
		for _, view := range views {
			mux.HandleFunc("GET "+view.path, admit(offering(view.has, whenThere(s.pageFound(view.path), s.appPage))))
			mux.HandleFunc("GET /app/api"+view.path, admit(offering(view.has, s.appData(view.gather))))
		}
	}
	mount(s.appViews(), s.console)
	mount(s.personViews(), s.guard)
	for _, view := range s.publicViews() {
		mux.HandleFunc("GET /app/api"+view.path, offering(view.has, s.appData(view.gather)))
	}
	mux.HandleFunc("GET /app/api/console", s.console(s.consoleJSON))
	mux.HandleFunc("GET /app/api/shell", s.guard(s.personShellJSON))
	mux.HandleFunc("GET /app/api"+conversationRoute+"/turns", s.guard(s.turnsData))
	mux.HandleFunc("GET /app/api/rooms/{id}/files", s.guard(s.roomFilesData))
	// A run's file is a page for a browser and its bytes for anybody else, so its page is served where its bytes are.
	mux.HandleFunc("GET /app/api"+recordFileRoute, s.guard(s.appData(s.recordFileData)))
	mux.Handle("GET /app/assets/", AppAssets())
}

// appHead is what the product's pages load before their own files: the shared head, and the script that keeps the page above a
// phone's keyboard. Behind gusto cloud it also loads the lease banner, which extends the host the organization runs on, and its
// guide.
func appHead(lease string) string {
	head := SharedHead() + `<script src="` + asset("/static/keyboard.js") + `" defer></script>`
	if lease != "" {
		head += `<script src="` + asset("/static/lease.js") + `" defer data-extend="` + html.EscapeString(lease) + `"></script>` +
			`<script src="` + asset("/static/guide.js") + `" defer></script>`
	}
	return head
}

// appPage is the React app's one page, which draws whichever of its pages the address names.
func (s *Server) appPage(w http.ResponseWriter, _ *http.Request) {
	WriteAppPage(w, http.StatusOK, ProductEntry, appHead(s.leaseExtend()), "")
}

// publicPage is the app's page for a page served before anybody is signed in. It loads nothing that a session gives, so not the
// lease banner, which asks for the host the organization runs on.
func (s *Server) publicPage(w http.ResponseWriter, _ *http.Request) {
	WriteAppPage(w, http.StatusOK, ProductEntry, appHead(""), "")
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
