package web

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/internal/workspace"
)

// A workspace is a team's space in the organization. Its tasks, records, conversations, agents, connectors and computers are
// reached only by its members, whatever their role in the organization. The workspace a request is in is where what it
// makes goes, and what its pages list; reaching one thing is decided by the workspace that thing is in.
//
// Gusto cloud says who is in which workspace, in the roster. A server that is not behind gusto cloud has one workspace, the
// first, and everybody who signs in is in it, as it holds a roster only for the names in it.

// workspaceCookie is where a browser keeps the workspace its person chose. It only chooses among the person's own.
const workspaceCookie = "gusto-workspace"

// workspaceCookieName is what the cookie is called over TLS, where a host name of a shared app cannot set it for this one.
func workspaceCookieName(r *http.Request) string {
	if overTLS(r) {
		return "__Host-" + workspaceCookie
	}
	return workspaceCookie
}

type workspaceKey struct{}

// tokenSpace is the workspace an organization's token works in, when the request carries one.
func tokenSpace(r *http.Request) (string, bool) {
	space, ok := r.Context().Value(workspaceKey{}).(string)
	return space, ok
}

func withTokenSpace(ctx context.Context, space string) context.Context {
	return context.WithValue(ctx, workspaceKey{}, workspace.Of(space))
}

// spacesFor is the workspaces a person is in. A server that is not behind gusto cloud, and the shared sign-in, have the first only.
func (s *Server) spacesFor(who string) []roster.Workspace {
	if s.front == nil || s.rosterPath == "" || who == sharedSignIn || who == "" {
		return []roster.Workspace{{ID: workspace.First}}
	}
	spaces, read := s.names.spacesOf(s.rosterPath, who)
	if !read {
		return nil
	}
	return spaces
}

// inWorkspace says whether the request may reach what is kept in a workspace.
func (s *Server) inWorkspace(r *http.Request, space string) bool {
	if held, ok := tokenSpace(r); ok {
		return held == workspace.Of(space)
	}
	return s.isIn(s.askerName(r), space)
}

// isIn says whether a person is in a workspace.
func (s *Server) isIn(who, space string) bool {
	space = workspace.Of(space)
	return slices.ContainsFunc(s.spacesFor(strings.ToLower(strings.TrimSpace(who))), func(w roster.Workspace) bool { return w.ID == space })
}

// workspaceOf is the workspace a request is in: an organization token's own, then the one the request names with
// ?workspace=, as a program with a person's token does, then the one the person chose in the switcher, each while they are
// in it, then the first they are in.
func (s *Server) workspaceOf(r *http.Request) string {
	if held, ok := tokenSpace(r); ok {
		return held
	}
	spaces := s.spacesFor(s.askerName(r))
	named := []string{r.URL.Query().Get("workspace")}
	if cookie, err := r.Cookie(workspaceCookieName(r)); err == nil {
		named = append(named, cookie.Value)
	}
	for _, want := range named {
		if want != "" && slices.ContainsFunc(spaces, func(w roster.Workspace) bool { return w.ID == want }) {
			return want
		}
	}
	if len(spaces) > 0 {
		return spaces[0].ID
	}
	return workspace.First
}

// chooseWorkspace makes one of the person's workspaces the one their pages are in, and sends them to its start.
func (s *Server) chooseWorkspace(w http.ResponseWriter, r *http.Request) {
	s.keepWorkspace(w, r, s.askerName(r), r.PathValue("id"))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// keepWorkspace remembers the workspace a person chose, when they are in it.
func (s *Server) keepWorkspace(w http.ResponseWriter, r *http.Request, who, space string) {
	if workspace.Valid(space) != nil || !s.isIn(who, space) {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: workspaceCookieName(r), Value: space, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: overTLS(r), MaxAge: 400 * 24 * 60 * 60,
	})
}

// sentInto is the workspace a task sent to the API is kept in: the one it names, when the sender is in it, and the sender's
// own otherwise. An organization's token sends only into its own.
func (s *Server) sentInto(r *http.Request, named string) (string, error) {
	if named == "" {
		return workspace.Stored(s.workspaceOf(r)), nil
	}
	if workspace.Valid(named) != nil || !s.inWorkspace(r, named) {
		return "", fmt.Errorf("the task names the workspace %q, which this sender is not in", named)
	}
	return workspace.Stored(named), nil
}

// outsideWorkspace is why a person cannot be put in a room, when they are not in the room's workspace, and nothing when they are.
func (s *Server) outsideWorkspace(held instance.Instance, who string) string {
	if s.isIn(who, held.Workspace) {
		return ""
	}
	return s.personName(who) + " is not in this room's workspace"
}

// shownWorkspace is the workspace a page names, and every workspace the person is in, for the switcher. A person in one
// workspace is shown none, since there is nothing to choose.
type shownWorkspace struct {
	Here  roster.Workspace
	Other []roster.Workspace
}

func (s *Server) shownWorkspaces(r *http.Request) *shownWorkspace {
	spaces := s.spacesFor(s.askerName(r))
	if len(spaces) < 2 {
		return nil
	}
	here := s.workspaceOf(r)
	shown := &shownWorkspace{}
	for _, space := range spaces {
		if space.ID == here {
			shown.Here = space
		} else {
			shown.Other = append(shown.Other, space)
		}
	}
	return shown
}
