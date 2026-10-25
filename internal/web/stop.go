package web

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/runner"
)

// stopTurn stops a job for whoever is asking, when they may. It writes down who before it tells the run to stop, so that the
// record the run leaves can say. It gives the job as it stands, whether it was running, and the status to answer with.
func (s *Server) stopTurn(r *http.Request, by string) (jobs.Record, bool, int) {
	record, found := s.index.Get(r.PathValue("id"))
	if !found || !s.mayUseRecord(r, record) {
		return jobs.Record{}, false, http.StatusNotFound
	}
	if record.State.Done() {
		return record, false, http.StatusConflict
	}
	// Before the stop, since a run that ends at once reads it as it ends. A job that cannot be written to is not stopped.
	if err := runner.NoteStop(record.Dir(s.index.Root()), by, time.Now().Unix()); err != nil {
		return record, false, http.StatusInternalServerError
	}
	stopped, wasRunning, err := s.queue.Cancel(record.ID, by)
	switch {
	case errors.Is(err, jobs.ErrNotStoppable):
		return stopped, false, http.StatusConflict
	case err != nil:
		return stopped, false, http.StatusInternalServerError
	}
	return stopped, wasRunning, http.StatusOK
}

// stopPage is the Stop button: a person stops one of their own turns and goes back to where they were.
func (s *Server) stopPage(w http.ResponseWriter, r *http.Request) {
	record, _, status := s.stopTurn(r, s.keeper.who(r))
	if status == http.StatusNotFound {
		http.NotFound(w, r)
		return
	}
	back := "/jobs/" + r.PathValue("id")
	if to := r.FormValue("back"); strings.HasPrefix(to, "/") && !strings.HasPrefix(to, "//") {
		back = to
	}
	if status == http.StatusConflict || status == http.StatusOK {
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	s.turnAway(w, r, back, "that turn could not be stopped: "+record.State.String())
}

// stopJSON is stopping a turn with a person's token. It answers 200 with the job as it stands: a queued turn is already
// interrupted, and a running one is being stopped and ends interrupted a moment later. A turn that is over answers 409.
func (s *Server) stopJSON(w http.ResponseWriter, r *http.Request) {
	record, wasRunning, status := s.stopTurn(r, personOf(r))
	switch status {
	case http.StatusNotFound:
		http.NotFound(w, r)
	case http.StatusOK:
		answerJSON(w, http.StatusOK, map[string]any{"job": record, "wasRunning": wasRunning})
	case http.StatusConflict:
		answerJSON(w, http.StatusConflict, map[string]any{"error": "that turn is already over", "job": record})
	default:
		answerJSON(w, status, map[string]string{"error": "that turn could not be stopped"})
	}
}
