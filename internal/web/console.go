package web

import "net/http"

// section is one entry in the console's navigation.
type section struct {
	Key, Href, Label string
}

// sections is the console's navigation. A section is listed only where this
// server has what it shows, so no entry leads to a page that says it is not
// there.
func (s *Server) sections() []section {
	all := []section{
		{"overview", "/admin", "Overview"},
		{"usage", "/admin/usage", "Usage"},
	}
	if s.hasRoster() {
		all = append(all, section{"people", "/admin/people", "People"})
	}
	if s.hasRegistry() {
		all = append(all, section{"models", "/admin/models", "Models"})
	}
	if s.hasAppsOrSkills() {
		all = append(all, section{"apps", appsPath, "Apps & skills"})
	}
	return append(all, section{"records", recordsPath, "Records"})
}

// console lets an admin through and answers 403 to anybody else who is signed
// in. The console is not a secret, so a runner is told it is not theirs.
func (s *Server) console(next http.HandlerFunc) http.HandlerFunc {
	return s.guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.admin(r) {
			http.Error(w, "the console is for an administrator", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

// offering answers 404 when the server has nothing for a section to show.
func offering(has func() bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !has() {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

func (s *Server) hasChanges() bool   { return s.changes != nil }
func (s *Server) hasTokens() bool    { return s.tokens != nil }
func (s *Server) hasRetention() bool { return s.retention != nil }
func (s *Server) hasSetups() bool    { return s.setups != nil }

// drawConsole draws a page of the console. key names the section it belongs to.
func (s *Server) drawConsole(w http.ResponseWriter, name, key string, page map[string]any) {
	page["Console"], page["Nav"], page["Sections"] = true, key, s.sections()
	if s.front != nil {
		page["Org"] = s.front.Org
		if s.front.Org != "" {
			page["OrgPage"] = s.front.page("/org", nil)
		}
	}
	s.render(w, name, page)
}

func (s *Server) consoleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin", s.console(s.overviewPage))
	mux.HandleFunc("GET /admin/spending", s.console(s.consoleSpend))
	mux.HandleFunc("GET "+recordsPath, s.console(s.recordsPage))
	mux.HandleFunc("GET "+appsPath, s.console(offering(s.hasAppsOrSkills, s.appsPage)))
	mux.HandleFunc("GET /admin/ui-kit", s.console(s.uiKitPage))

	changes := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasChanges, next)) }
	mux.HandleFunc("GET /admin/activity.jsonl", changes(s.changesFile))

	tokens := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasTokens, next)) }
	mux.HandleFunc("GET /admin/tokens", tokens(s.tokensPage))
	mux.HandleFunc("POST /admin/tokens", tokens(s.makeToken))
	mux.HandleFunc("POST /admin/tokens/{id}/revoke", tokens(s.revokeToken))

	retention := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasRetention, next)) }
	mux.HandleFunc("POST /admin/retention/sweep", retention(s.sweepNow))
	mux.HandleFunc("POST /admin/retention/holds", retention(s.holdPerson))
	mux.HandleFunc("POST /admin/retention/holds/release", retention(s.releasePerson))
	mux.HandleFunc("POST /admin/retention/jobs/hold", retention(s.holdJob))
	mux.HandleFunc("POST /admin/retention/jobs/release", retention(s.releaseJob))

	setups := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasSetups, next)) }
	mux.HandleFunc("POST "+setupsPath, setups(s.saveSetup))
	mux.HandleFunc("GET "+setupsPath+"/{name}", setups(s.skillSetPage))
	mux.HandleFunc("GET "+setupsPath+"/{name}/plugin.zip", setups(s.exportSetup))
	mux.HandleFunc("POST "+setupsPath+"/{name}/skills", setups(s.writeSkill))
	mux.HandleFunc("POST "+setupsPath+"/{name}/skills/{skill}/drop", setups(s.dropSkill))
	mux.HandleFunc("POST "+setupsPath+"/{name}/skills/{skill}/restore", setups(s.restoreSkill))
	mux.HandleFunc("POST "+setupsPath+"/{name}/enable", setups(s.turnSetup(false)))
	mux.HandleFunc("POST "+setupsPath+"/{name}/disable", setups(s.turnSetup(true)))

	mux.HandleFunc("GET /admin/people", s.console(offering(s.hasRoster, s.peoplePage)))
	mux.HandleFunc("POST /admin/budget", s.console(offering(s.hasOrgSettings, s.setBudget)))
	mux.HandleFunc("POST /admin/member-budget", s.console(offering(s.hasOrgSettings, s.setMemberBudget)))
	mux.HandleFunc("POST /admin/chat-default", s.console(offering(s.hasOrgSettings, s.setChatDefault)))
	mux.HandleFunc("POST /admin/run-tokens", s.console(offering(s.hasOrgSettings, s.setRunTokens)))

	s.integrationRoutes(mux)
	s.modelRoutes(mux)
	s.movedRoutes(mux)
}

// movedRoutes answer at the addresses a bookmark or a form left open across an
// update names. A page goes to its place in the console, and a form is sent again
// to the same place with its method kept.
func (s *Server) movedRoutes(mux *http.ServeMux) {
	for from, to := range map[string]string{
		"/changes":       activityTab,
		"/changes.jsonl": "/admin/activity.jsonl",
		"/tokens":        "/admin/tokens",
		"/retention":     keepingTab,
	} {
		mux.HandleFunc("GET "+from, s.guard(moved(to)))
	}
	// A page that became a tab of another goes to that tab, with what it was sent to say.
	for from, to := range map[string]string{
		"/admin/activity":     activityTab,
		"/admin/verification": checksTab,
		"/admin/retention":    keepingTab,
		"/admin/connectors":   appsTab,
		setupsPath:            skillsTab,
		integrationsPath:      skillsTab,
	} {
		mux.HandleFunc("GET "+from, s.console(toTab(to)))
	}
	for from, to := range map[string]string{
		"/tokens":                            "/admin/tokens",
		"/tokens/{id}/revoke":                "/admin/tokens/{id}/revoke",
		"/retention/sweep":                   "/admin/retention/sweep",
		"/retention/holds":                   "/admin/retention/holds",
		"/retention/holds/release":           "/admin/retention/holds/release",
		"/setups":                            setupsPath,
		"/setups/{name}/skills":              setupsPath + "/{name}/skills",
		"/setups/{name}/skills/{skill}/drop": setupsPath + "/{name}/skills/{skill}/drop",
	} {
		mux.HandleFunc("POST "+from, s.guard(redirected(to, http.StatusPermanentRedirect)))
	}
}

// toTab sends a request for a page that is now a tab to that tab, keeping its query.
func toTab(tab string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		to := tab
		if r.URL.RawQuery != "" {
			to += "&" + r.URL.RawQuery
		}
		http.Redirect(w, r, to, http.StatusSeeOther)
	}
}

// tab is one view of a console page, as a link.
type tab struct {
	Key, Label, Href string
	On               bool
}

// chooseTab marks the tab the request names, or the first when it names none of them, and says which it is.
func chooseTab(r *http.Request, path string, tabs []tab) ([]tab, string) {
	chosen := tabs[0].Key
	for _, each := range tabs {
		if each.Key == r.URL.Query().Get("tab") {
			chosen = each.Key
		}
	}
	for i := range tabs {
		tabs[i].Href, tabs[i].On = path+"?tab="+tabs[i].Key, tabs[i].Key == chosen
	}
	return tabs, chosen
}
