package web

import (
	"cmp"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/connector"
	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/orgsettings"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/pkg/gateway"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// connectionsPath is where a person signs in to the apps their agents may use as them.
const connectionsPath = "/connections"

// UseConnectors hands the pages the store of people's sign-ins to apps. A server without it, or without settings to say which
// apps are on, offers none.
func (s *Server) UseConnectors(store *connector.Connectors) { s.connectors = store }

func (s *Server) hasConnectors() bool { return s.connectors != nil && s.orgSettings != nil }

func (s *Server) connectorRoutes(mux *http.ServeMux) {
	// An app reads the client document to know this server, and has no sign-in here.
	mux.HandleFunc("GET /connectors/client.json", offering(s.hasConnectors, s.clientDocument))
	mine := func(next http.HandlerFunc) http.HandlerFunc { return s.guard(offering(s.hasConnectors, next)) }
	mux.HandleFunc("GET "+connectionsPath, mine(s.connectionsPage))
	mux.HandleFunc("GET "+connectionsPath+"/callback", mine(s.finishConnecting))
	// Connecting is a link and not a form: the page it leads to is the app's, and a form may only lead to this server.
	mux.HandleFunc("GET "+connectionsPath+"/{id}/connect", mine(s.connect))
	mux.HandleFunc("POST "+connectionsPath+"/{id}/disconnect", mine(s.disconnect))
	mux.HandleFunc("POST /members/agents/{id}/apps", mine(s.chooseAgentApp))
	mux.HandleFunc("POST "+connectionsPath+"/{id}/agents", mine(s.setAppForAgents))
	admin := func(next http.HandlerFunc) http.HandlerFunc { return s.console(offering(s.hasConnectors, next)) }
	mux.HandleFunc("POST /admin/connectors/{id}/on", admin(s.turnConnector(true)))
	mux.HandleFunc("POST /admin/connectors/{id}/off", admin(s.turnConnector(false)))
}

func (s *Server) clientDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(s.connectors.ClientDocument())
}

// connectorView is one connector as a page shows it.
type connectorView struct {
	connector.Entry
	On, Usable, Connected bool
	Since                 time.Time
	// ForAgents is the person's default for whether their agents may use the app.
	ForAgents bool
}

// appAllowed says whether a task may be given one app of its asker's: any, on their own task that they pay for; on a turn they
// asked of a room's agent, what they chose for that agent and app, or else their default for their agents.
func (s *Server) appAllowed(record jobs.Record, app string) bool {
	if !record.InRoom() {
		return record.Payer() == record.Asked
	}
	member, found := s.memberOf(record)
	if !found {
		return false
	}
	if allowed, chosen := member.AppChoice(record.Asked, app); chosen {
		return allowed
	}
	return s.connectors.ForAgents(record.Asked, app)
}

// memberOf is the workspace agent that took a room turn.
func (s *Server) memberOf(record jobs.Record) (crew.Agent, bool) {
	if s.instances == nil || s.crew == nil {
		return crew.Agent{}, false
	}
	held, found := s.instances.Get(record.Instance)
	if !found || !held.Room {
		return crew.Agent{}, false
	}
	agent, found := held.AgentByID(record.Agent)
	if !found || agent.Member == "" {
		return crew.Agent{}, false
	}
	return s.crew.Get(agent.Member)
}

// connectorViews is every connector in the catalog, as it stands for one person, or for nobody on the admin's page.
func (s *Server) connectorViews(person string) ([]connectorView, error) {
	settings, err := s.orgSettings.Read()
	if err != nil {
		return nil, err
	}
	views := make([]connectorView, 0, len(s.connectors.Apps()))
	for _, entry := range s.connectors.Apps() {
		view := connectorView{Entry: entry, On: settings.ConnectorOn(entry.ID), Usable: s.connectors.Usable(entry)}
		if person != "" {
			view.Since, view.Connected = s.connectors.Connected(person, entry.ID)
			view.ForAgents = view.Connected && s.connectors.ForAgents(person, entry.ID)
		}
		views = append(views, view)
	}
	return views, nil
}

// connectingPerson is who is signed in, when that is a person: the shared sign-in is the operator's and nobody's own, and has no
// accounts at apps to give.
func (s *Server) connectingPerson(r *http.Request) (string, bool) {
	who := s.keeper.who(r)
	return who, who != "" && who != sharedSignIn
}

func (s *Server) connectionsPage(w http.ResponseWriter, r *http.Request) {
	who, person := s.connectingPerson(r)
	views, err := s.connectorViews(who)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	offered := slices.DeleteFunc(views, func(view connectorView) bool { return !view.On || !view.Usable })
	s.render(w, "connections.html", map[string]any{
		"Title": productName + " — connections", "Nav": "connections", "Connectors": offered, "Person": person,
		"Refused": r.URL.Query().Get("refused"), "Done": r.URL.Query().Get("connected"),
	})
}

// offeredConnector is the connector of that id, when an admin has turned it on and people can sign in to it.
func (s *Server) offeredConnector(id string) (connector.Entry, bool) {
	entry, known := s.connectors.App(id)
	if !known || !s.connectors.Usable(entry) {
		return connector.Entry{}, false
	}
	settings, err := s.orgSettings.Read()
	return entry, err == nil && settings.ConnectorOn(entry.ID)
}

// connect sends a person to the app to sign in, with their own account there.
func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	entry, offered := s.offeredConnector(r.PathValue("id"))
	if !offered {
		http.NotFound(w, r)
		return
	}
	who, person := s.connectingPerson(r)
	if !person {
		s.turnAway(w, r, connectionsPath, "Sign in as yourself to connect your own accounts: the shared sign-in has none.")
		return
	}
	address, err := s.connectors.Begin(r.Context(), who, entry)
	if err != nil {
		s.turnAway(w, r, connectionsPath, "Signing in to "+entry.Name+" could not start: "+err.Error())
		return
	}
	http.Redirect(w, r, address, http.StatusSeeOther)
}

// finishConnecting is where the app sends a person back to, with a code for their tokens, or why it did not sign them in.
func (s *Server) finishConnecting(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if why := query.Get("error"); why != "" {
		s.turnAway(w, r, connectionsPath, "The app did not sign you in: "+cmp.Or(query.Get("error_description"), why))
		return
	}
	id, err := s.connectors.Finish(r.Context(), s.keeper.who(r), query.Get("state"), query.Get("code"))
	if err != nil {
		s.turnAway(w, r, connectionsPath, err.Error())
		return
	}
	// A connection starts from the person's default for their agents, and a disconnect made before choices were forgotten with it
	// left some.
	if err := s.forgetAgentChoices(s.keeper.who(r), id); err != nil {
		s.turnAway(w, r, connectionsPath, err.Error())
		return
	}
	entry, _ := s.connectors.App(id)
	s.note(r, changelog.ConnectorConnected, map[string]string{"connector": entry.Name})
	http.Redirect(w, r, connectionsPath+"?connected="+url.QueryEscape(entry.Name), http.StatusSeeOther)
}

func (s *Server) disconnect(w http.ResponseWriter, r *http.Request) {
	entry, known := s.connectors.App(r.PathValue("id"))
	if !known {
		http.NotFound(w, r)
		return
	}
	// What the person chose for their agents goes first, so a disconnect that fails leaves them connected on their default, and
	// never ends a connection whose choices a later one would find.
	who := s.keeper.who(r)
	if err := s.forgetAgentChoices(who, entry.ID); err != nil {
		s.turnAway(w, r, connectionsPath, err.Error())
		return
	}
	if err := s.connectors.Disconnect(who, entry.ID); err != nil {
		s.turnAway(w, r, connectionsPath, err.Error())
		return
	}
	s.note(r, changelog.ConnectorDisconnected, map[string]string{"connector": entry.Name})
	http.Redirect(w, r, connectionsPath, http.StatusSeeOther)
}

// turnConnector lets people sign in to a connector and give it to their agents, or stops them: their agents are not given it
// from their next task, and what they signed in to stays theirs to end.
func (s *Server) turnConnector(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entry, known := s.connectors.App(r.PathValue("id"))
		if !known {
			http.NotFound(w, r)
			return
		}
		if on && !s.connectors.Usable(entry) {
			s.turnAway(w, r, appsTab, entry.Name+" needs setting up by whoever runs this server first")
			return
		}
		changed := false
		err := s.orgSettings.Change(func(settings *orgsettings.Settings) {
			changed = settings.ConnectorOn(entry.ID) != on
			settings.Connectors = slices.DeleteFunc(settings.Connectors, func(id string) bool { return id == entry.ID })
			if on {
				settings.Connectors = append(settings.Connectors, entry.ID)
			}
		})
		if err != nil {
			s.turnAway(w, r, appsTab, err.Error())
			return
		}
		if changed {
			kind, now := changelog.ConnectorDisabled, "off"
			if on {
				kind, now = changelog.ConnectorEnabled, "on"
			}
			s.note(r, kind, map[string]string{"connector": entry.Name, "now": now})
		}
		back := appsTab
		if on {
			back += "&on=" + url.QueryEscape(entry.Name)
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

// giveConnectors puts the apps the asker signed in to behind the task's gateway, so their agent calls them as the asker. A
// person's own task is given them. A room's agent works for everybody in the room, so a room turn is given them only when its
// asker has let that agent use an app, and then only the asker's apps they let it use. An agent that cannot reach the gateway is given
// none, and runs as it would without them.
func (s *Server) giveConnectors(spec *job.Spec, dir job.Dir, record jobs.Record) error {
	if !s.hasConnectors() {
		return nil
	}
	settings, err := s.orgSettings.Read()
	if err != nil {
		return err
	}
	if adapter, err := harness.Load(dir, spec.Harness); err != nil || !adapter.ReachesGateway() {
		return nil
	}
	var taken []string
	if spec.Setup != nil {
		taken = spec.Setup.Gateway
	}
	var servers []gateway.Server
	var given []string
	for _, entry := range s.connectors.Apps() {
		if !settings.ConnectorOn(entry.ID) || slices.Contains(taken, entry.ID) || !s.appAllowed(record, entry.ID) {
			continue
		}
		if _, connected := s.connectors.Connected(record.Asked, entry.ID); connected {
			servers = append(servers, gateway.Server{Name: entry.ID, URL: entry.URL, Connector: entry.ID, Person: record.Asked})
			given = append(given, entry.ID)
		}
	}
	if len(given) == 0 {
		return nil
	}
	if spec.Setup == nil {
		spec.Setup = &job.Setup{}
	}
	spec.Setup.Gateway = append(spec.Setup.Gateway, given...)
	spec.Setup.Connectors = given
	return setup.AddToGateway(dir, servers)
}
