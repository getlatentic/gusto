package web

import (
	"net/http"
)

// connectionRow is one app on a person's Connections page: what it is, whether they are signed in to it or must sign in again,
// and whether their agents may use it. Offered is an app an admin has turned on; one that is not is listed only while the person
// still holds a sign-in to it, so they can end it.
type connectionRow struct {
	ID, Name, What                  string
	Offered, Connected, SignInAgain bool
	ForAgents                       bool
	// Since is when the sign-in was made, in Unix seconds.
	Since int64
}

// connectionsData is a person's Connections page: the apps their agents may use as them, what became of the last thing they
// asked, and the home their agents keep. The shared sign-in is nobody's own, so it holds no sign-ins and has no home.
func (s *Server) connectionsData(r *http.Request) (map[string]any, error) {
	who, person := s.connectingPerson(r)
	space := s.workspaceOf(r)
	views, err := s.connectorViews(who, space)
	if err != nil {
		return nil, failed{http.StatusInternalServerError, err.Error()}
	}
	rows := []connectionRow{}
	for _, view := range views {
		if view.Offered() || view.SignedInTo() {
			rows = append(rows, connectionRowOf(view))
		}
	}
	var home *connectionsHome
	if person {
		home = s.connectionsHomeOf(who, space)
	}
	return map[string]any{
		"Person": person, "Connectors": rows, "Refused": s.refusal(r), "Done": s.message(r, "connected"),
		"Home": home, "HomeReset": s.message(r, "reset") != "",
	}, nil
}

func connectionRowOf(view connectorView) connectionRow {
	row := connectionRow{ID: view.ID, Name: view.Name, What: view.What, Offered: view.Offered(), Connected: view.Connected,
		SignInAgain: view.SignInAgain, ForAgents: view.ForAgents}
	if !view.Since.IsZero() {
		row.Since = view.Since.Unix()
	}
	return row
}
