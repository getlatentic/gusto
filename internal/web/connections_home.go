package web

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/getlatentic/gusto/internal/backend/container"
)

// connectionsHomePath is where a person resets the home their agents keep between tasks.
const connectionsHomePath = connectionsPath + "/home/reset"

// KeptHomes is a backend that keeps a home for each person in each workspace, which an agent's tasks reuse and which has a size
// limit.
type KeptHomes interface {
	KeepsHomes() bool
	HomeUse(person, space string) (container.HomeUse, bool)
	ResetHome(person, space string) error
}

// UseHomes lets the pages say how much of its limit a person's home takes, and lets the person reset it.
func (s *Server) UseHomes(homes KeptHomes) { s.keptHomes = homes }

func (s *Server) hasKeptHomes() bool { return s.keptHomes != nil && s.keptHomes.KeepsHomes() }

// connectionsHome is a person's home as Connections shows it.
type connectionsHome struct {
	// Counted is false until a task of theirs has ended in it.
	Counted     bool
	Used, Limit string
	// Full is a home over its limit, which refuses their tasks until it is reset.
	Full bool
	At   time.Time
}

// connectionsHomeOf is the home of the person asking in the workspace they are in, or nil when this server keeps none or the
// shared sign-in, which is nobody, is asking.
func (s *Server) connectionsHomeOf(person, space string) *connectionsHome {
	if !s.hasKeptHomes() || person == "" {
		return nil
	}
	use, counted := s.keptHomes.HomeUse(person, space)
	if !counted {
		return &connectionsHome{}
	}
	return &connectionsHome{Counted: true, Used: homeSize(use.Used), Limit: homeSize(use.Limit), Full: use.Used > use.Limit, At: use.At}
}

// resetHome removes the home of the person asking, which is how a person whose home is full starts again: a task cannot free
// it while it is refused.
func (s *Server) resetHome(w http.ResponseWriter, r *http.Request) {
	who, person := s.connectingPerson(r)
	if !person || !s.hasKeptHomes() {
		http.NotFound(w, r)
		return
	}
	switch err := s.keptHomes.ResetHome(who, s.workspaceOf(r)); {
	case errors.Is(err, container.ErrHomeBusy):
		s.turnAway(w, r, connectionsPath, "Your agents are working in it now. Reset it when their task has ended.")
	case err != nil:
		s.turnAway(w, r, connectionsPath, "The home could not be reset: "+err.Error())
	default:
		http.Redirect(w, r, s.withMessage(connectionsPath, "reset", "home"), http.StatusSeeOther)
	}
}

// homeSize is a number of bytes in the words a person reads.
func homeSize(bytes int64) string {
	const unit = 1 << 10
	value, suffix := float64(bytes), "bytes"
	for _, next := range []string{"KiB", "MiB", "GiB", "TiB"} {
		if value < unit {
			break
		}
		value, suffix = value/unit, next
	}
	if suffix == "bytes" {
		return fmt.Sprintf("%d bytes", bytes)
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}
