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
	// The UI kit is the pages' own reference, kept at its address for whoever builds them and out of an admin's menu.
	all = append(all, section{"verification", "/admin/verification", "Verification"})
	if s.changes != nil {
		all = append(all, section{"activity", "/admin/activity", "Activity"})
	}
	if s.tokens != nil {
		all = append(all, section{"tokens", "/admin/tokens", "Tokens"})
	}
	if s.retention != nil {
		all = append(all, section{"retention", "/admin/retention", "Retention"})
	}
	if s.setups != nil {
		all = append(all, section{"skill-sets", "/admin/skill-sets", "Skill sets"})
	}
	if s.hasIntegrations() {
		all = append(all, section{"integrations", integrationsPath, "Integrations"})
	}
	if s.hasConnectors() {
		all = append(all, section{"connectors", "/admin/connectors", "Connectors"})
	}
	if s.hasRoster() {
		all = append(all, section{"people", "/admin/people", "People"})
	}
	if s.hasRegistry() {
		all = append(all, section{"models", "/admin/models", "Models"})
	}
	return all
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
	}
	s.render(w, name, page)
}

func (s *Server) consoleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin", s.console(s.overviewPage))
	mux.HandleFunc("GET /admin/spending", s.console(s.consoleSpend))
	mux.HandleFunc("GET /admin/verification", s.console(s.verificationPage))
	mux.HandleFunc("GET /admin/ui-kit", s.console(s.uiKitPage))

	changes := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasChanges, next)) }
	mux.HandleFunc("GET /admin/activity", changes(s.changesPage))
	mux.HandleFunc("GET /admin/activity.jsonl", changes(s.changesFile))

	tokens := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasTokens, next)) }
	mux.HandleFunc("GET /admin/tokens", tokens(s.tokensPage))
	mux.HandleFunc("POST /admin/tokens", tokens(s.makeToken))
	mux.HandleFunc("POST /admin/tokens/{id}/revoke", tokens(s.revokeToken))

	retention := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasRetention, next)) }
	mux.HandleFunc("GET /admin/retention", retention(s.retentionPage))
	mux.HandleFunc("POST /admin/retention/sweep", retention(s.sweepNow))
	mux.HandleFunc("POST /admin/retention/holds", retention(s.holdPerson))
	mux.HandleFunc("POST /admin/retention/holds/release", retention(s.releasePerson))
	mux.HandleFunc("POST /admin/retention/jobs/hold", retention(s.holdJob))
	mux.HandleFunc("POST /admin/retention/jobs/release", retention(s.releaseJob))

	setups := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasSetups, next)) }
	mux.HandleFunc("GET /admin/skill-sets", setups(s.skillSetsPage))
	mux.HandleFunc("POST /admin/skill-sets", setups(s.saveSetup))
	mux.HandleFunc("GET /admin/skill-sets/{name}", setups(s.skillSetPage))
	mux.HandleFunc("GET /admin/skill-sets/{name}/plugin.zip", setups(s.exportSetup))
	mux.HandleFunc("POST /admin/skill-sets/{name}/skills", setups(s.writeSkill))
	mux.HandleFunc("POST /admin/skill-sets/{name}/skills/{skill}/drop", setups(s.dropSkill))
	mux.HandleFunc("POST /admin/skill-sets/{name}/skills/{skill}/restore", setups(s.restoreSkill))
	mux.HandleFunc("POST /admin/skill-sets/{name}/enable", setups(s.turnSetup(false)))
	mux.HandleFunc("POST /admin/skill-sets/{name}/disable", setups(s.turnSetup(true)))

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
		"/changes":       "/admin/activity",
		"/changes.jsonl": "/admin/activity.jsonl",
		"/tokens":        "/admin/tokens",
		"/retention":     "/admin/retention",
	} {
		mux.HandleFunc("GET "+from, s.guard(moved(to)))
	}
	for from, to := range map[string]string{
		"/tokens":                            "/admin/tokens",
		"/tokens/{id}/revoke":                "/admin/tokens/{id}/revoke",
		"/retention/sweep":                   "/admin/retention/sweep",
		"/retention/holds":                   "/admin/retention/holds",
		"/retention/holds/release":           "/admin/retention/holds/release",
		"/setups":                            "/admin/skill-sets",
		"/setups/{name}/skills":              "/admin/skill-sets/{name}/skills",
		"/setups/{name}/skills/{skill}/drop": "/admin/skill-sets/{name}/skills/{skill}/drop",
	} {
		mux.HandleFunc("POST "+from, s.guard(redirected(to, http.StatusPermanentRedirect)))
	}
}
