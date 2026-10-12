package web

import (
	"net/http"

	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/isolation"
)

// Wall is the wall between the agents and the person's files on their own computer, and whether they have said they know there is
// none.
type Wall struct {
	Choice isolation.Choice
	// Accepted says the person has confirmed, once, that agents run with nothing between them and their files.
	Accepted func() bool
	// Accept keeps that confirmation.
	Accept func() error
}

// UseWall shows the page which wall agents run behind, and holds the first agent until a person with none has said they know.
func (s *Server) UseWall(wall Wall) { s.wall = &wall }

// unconfirmed says agents would run with nothing between them and the files, and the person has not yet said they know.
func (s *Server) unconfirmed() bool {
	return s.wall != nil && s.wall.Choice.Kind == isolation.Nothing && !s.wall.Accepted()
}

// wallView is what the chat page says of the wall: in a few words, in full, and what stronger walls this computer lacks.
func (s *Server) wallView() map[string]any {
	if s.wall == nil {
		return nil
	}
	missing := make([]string, 0, len(s.wall.Choice.Passed))
	for _, backend := range s.wall.Choice.Passed {
		missing = append(missing, backend.Detail)
	}
	return map[string]any{
		"Short": s.wall.Choice.Short(), "Words": s.wall.Choice.Words(), "Missing": missing, "Unconfirmed": s.unconfirmed(),
	}
}

// confirmNoWall keeps the person's word that they know agents run with no sandbox, and goes back to the chat.
func (s *Server) confirmNoWall(w http.ResponseWriter, r *http.Request) {
	if s.wall == nil || s.wall.Choice.Kind != isolation.Nothing {
		http.NotFound(w, r)
		return
	}
	if err := s.wall.Accept(); err != nil {
		s.turnAway(w, r, "/chat", err.Error())
		return
	}
	s.note(r, changelog.SandboxConfirmed, map[string]string{"sandbox": "none"})
	http.Redirect(w, r, "/chat", http.StatusSeeOther)
}
