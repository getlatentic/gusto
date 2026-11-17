package web

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/internal/team"
)

// A team is a group of the organization's people, such as Engineering or Business. Its tasks, records, conversations, agents, connectors and computers are
// reached only by its members, whatever their role in the organization. The team a request is in is where what it
// makes goes, and what its pages list; reaching one thing is decided by the team that thing is in.
//
// Gusto cloud says who is in which team, in the roster. A server that is not behind gusto cloud has one team, the
// first, and everybody who signs in is in it, as it holds a roster only for the names in it.

// teamCookie is where a browser keeps the team its person chose. It only chooses among the person's own.
const teamCookie = "gusto-team"

// teamCookieName is what the cookie is called for a request: over TLS, a name a host name of a shared app cannot set
// for this one.
func (s *Server) teamCookieName(r *http.Request) string {
	return lockedName(r, s.cookieBase(r), teamCookie)
}

type teamKey struct{}

// tokenSpace is the team an organization's token works in, when the request carries one.
func tokenSpace(r *http.Request) (string, bool) {
	space, ok := r.Context().Value(teamKey{}).(string)
	return space, ok
}

func withTokenSpace(ctx context.Context, space string) context.Context {
	return context.WithValue(ctx, teamKey{}, team.Of(space))
}

// spacesFor is the teams a person is in. A server that is not behind gusto cloud, and the shared sign-in, have the first only.
func (s *Server) spacesFor(who string) []roster.Team {
	if s.front == nil || s.rosterPath == "" || who == sharedSignIn || who == "" {
		return []roster.Team{{ID: team.First}}
	}
	spaces, read := s.names.spacesOf(s.rosterPath, who)
	if !read {
		return nil
	}
	return spaces
}

// inTeam says whether the request may reach what is kept in a team.
func (s *Server) inTeam(r *http.Request, space string) bool {
	if held, ok := tokenSpace(r); ok {
		return held == team.Of(space)
	}
	return s.isIn(s.askerName(r), space)
}

// isIn says whether a person is in a team.
func (s *Server) isIn(who, space string) bool {
	space = team.Of(space)
	return slices.ContainsFunc(s.spacesFor(strings.ToLower(strings.TrimSpace(who))), func(w roster.Team) bool { return w.ID == space })
}

// teamOf is the team a request is in: an organization token's own, then the one the request names with
// ?team= (or ?workspace=, its earlier name), as a program with a person's token does, then the one the person chose in the switcher, each while they are
// in it, then the first they are in.
func (s *Server) teamOf(r *http.Request) string {
	if held, ok := tokenSpace(r); ok {
		return held
	}
	spaces := s.spacesFor(s.askerName(r))
	named := []string{cmp.Or(r.URL.Query().Get("team"), r.URL.Query().Get("workspace"))}
	if cookie, err := r.Cookie(s.teamCookieName(r)); err == nil {
		named = append(named, cookie.Value)
	}
	for _, want := range named {
		if want != "" && slices.ContainsFunc(spaces, func(w roster.Team) bool { return w.ID == want }) {
			return want
		}
	}
	if len(spaces) > 0 {
		return spaces[0].ID
	}
	return team.First
}

// chooseTeam makes one of the person's teams the one their pages are in, and sends them to its start.
func (s *Server) chooseTeam(w http.ResponseWriter, r *http.Request) {
	s.keepTeam(w, r, s.askerName(r), r.PathValue("id"))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// keepTeam remembers the team a person chose, when they are in it.
func (s *Server) keepTeam(w http.ResponseWriter, r *http.Request, who, space string) {
	if team.Valid(space) != nil || !s.isIn(who, space) {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: s.teamCookieName(r), Value: space, Path: cookiePath(s.cookieBase(r), "/"), HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: overTLS(r), MaxAge: 400 * 24 * 60 * 60,
	})
}

// sentInto is the team a task sent to the API is kept in: the one it names, when the sender is in it, and the sender's
// own otherwise. An organization's token sends only into its own.
func (s *Server) sentInto(r *http.Request, named string) (string, error) {
	if named == "" {
		return team.Stored(s.teamOf(r)), nil
	}
	if team.Valid(named) != nil || !s.inTeam(r, named) {
		return "", fmt.Errorf("the task names the team %q, which this sender is not in", named)
	}
	return team.Stored(named), nil
}

// outsideTeam is why a person cannot be put in a room, when they are not in the room's team, and nothing when they are.
func (s *Server) outsideTeam(held instance.Instance, who string) string {
	if s.isIn(who, held.Team) {
		return ""
	}
	return s.personName(who) + " is not in this room's team"
}

// shownTeam is the team a page names, and every team the person is in, for the switcher. A person in one
// team is shown none, since there is nothing to choose.
type shownTeam struct {
	Here  teamName
	Other []teamName
}

// teamName is a team as the switcher shows it: its id and name, and not who is in it.
type teamName struct{ ID, Name string }

func (s *Server) shownTeams(r *http.Request) *shownTeam {
	spaces := s.spacesFor(s.askerName(r))
	if len(spaces) < 2 {
		return nil
	}
	here := s.teamOf(r)
	shown := &shownTeam{}
	for _, space := range spaces {
		named := teamName{ID: space.ID, Name: space.Name}
		if space.ID == here {
			shown.Here = named
		} else {
			shown.Other = append(shown.Other, named)
		}
	}
	return shown
}
