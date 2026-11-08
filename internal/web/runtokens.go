package web

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/orgsettings"
)

// runTokens is how a run in a sandbox reaches its model, as the models page shows it.
type runTokens struct {
	Bound bool
	Err   string
}

func (s *Server) runTokensOf() runTokens {
	settings, err := s.orgSettings.Read()
	if err != nil {
		return runTokens{Bound: true, Err: err.Error()}
	}
	return runTokens{Bound: !settings.PlainRunTokens}
}

// setRunTokens keeps whether a run in a sandbox gets a token bound to its sandbox, given as bound=yes or bound=no.
func (s *Server) setRunTokens(w http.ResponseWriter, r *http.Request) {
	var bound bool
	switch r.FormValue("bound") {
	case "yes":
		bound = true
	case "no":
	default:
		s.turnAway(w, r, s.settingsBack(), "say whether runs are bound: yes or no")
		return
	}
	var was bool
	err := s.orgSettings.Change(func(settings *orgsettings.Settings) {
		was = !settings.PlainRunTokens
		settings.PlainRunTokens = !bound
	})
	if err != nil {
		s.turnAway(w, r, s.settingsBack(), err.Error())
		return
	}
	if was != bound {
		s.note(r, changelog.RunTokensSet, map[string]string{"was": boundWord(was), "now": boundWord(bound)})
	}
	http.Redirect(w, r, s.settingsBack(), http.StatusSeeOther)
}

func boundWord(bound bool) string {
	if bound {
		return "bound"
	}
	return "plain"
}
