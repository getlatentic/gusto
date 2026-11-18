package cloud

import (
	"encoding/json"
	"net/http"
	"time"
)

// watchMost is how long a page waiting on its organization's server is kept waiting for an answer: under the minute after which a
// proxy on the way may end a quiet request.
const watchMost = 25 * time.Second

// watch is what a page that waits on an organization's server shows of it: the state it was drawn in, which it waits to change.
type watch struct {
	State State
}

// serverState answers a page waiting on the person's organization's server with that server's state once it is not the one the
// page was drawn in, or the same state after watchMost, so the page is drawn again as soon as there is something new to show.
func (s *Server) serverState(w http.ResponseWriter, r *http.Request, person Person) {
	if s.Controller == nil {
		http.NotFound(w, r)
		return
	}
	state := s.Controller.AwaitChange(r.Context(), person.Org, State(r.URL.Query().Get("seen")), watchMost)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(watch{State: state})
}
