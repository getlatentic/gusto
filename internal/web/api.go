package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/rules"
)

// Submission is a job directory on its way to a server: the spec, and the
// adapter the directory carries. Nothing else has to cross, because the runner
// makes workspace/ and out/ and clones the repository itself.
type Submission struct {
	Spec job.Spec `json:"spec"`
	// The adapter is how a job for a harness this server has never heard of
	// still runs here. A job naming a built-in one carries none.
	Adapter *harness.Adapter `json:"adapter,omitempty"`
}

// Answer is the id of the job this server took, or the reason it took none.
type Answer struct {
	ID      string `json:"id,omitempty"`
	Refused string `json:"refused,omitempty"`
}

// sent guards an endpoint a program calls. It takes the same shared password the
// pages take, from the Authorization header rather than the cookie: no script
// can fill in a sign-in page, and the cookie's signing key is made at boot, so a
// saved cookie dies with every restart while the password does not. Accepting
// nothing a browser sends by itself also leaves no way to post from another
// origin on a signed-in person's behalf.
func (s *Server) sent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		given, bearing := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !bearing || !s.keeper.admits(given) {
			// Saying which of the two went wrong tells whoever sent this
			// nothing they did not already send, and saves them looking for a
			// header they wrote correctly.
			reason := "that is not the shared password"
			if !bearing {
				reason = "the shared password goes in an Authorization: Bearer header"
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="haas"`)
			reply(w, http.StatusUnauthorized, Answer{Refused: reason})
			return
		}
		next(w, r)
	}
}

// accept queues a job somebody made somewhere else, and refuses by name anything
// the runner would refuse: whoever sent it is at a terminal waiting for an
// answer, and a job that cannot run should not become a failed job to go and
// read.
func (s *Server) accept(w http.ResponseWriter, r *http.Request) {
	var sent Submission
	// A spec is a few hundred bytes, so a body that is not one is refused
	// rather than held in memory. A field this server does not know is refused
	// too: a spec written for a newer runner would otherwise run here with part
	// of itself quietly dropped.
	body := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	body.DisallowUnknownFields()
	if err := body.Decode(&sent); err != nil {
		reply(w, http.StatusBadRequest, Answer{Refused: "this is not a job spec: " + err.Error()})
		return
	}
	if short := missing(sent.Spec); short != "" {
		reply(w, http.StatusBadRequest, Answer{Refused: short})
		return
	}
	if err := sent.Spec.Budget.Check(); err != nil {
		reply(w, http.StatusBadRequest, Answer{Refused: err.Error()})
		return
	}
	adapter, err := runnable(sent.Spec, sent.Adapter)
	if err != nil {
		reply(w, http.StatusBadRequest, Answer{Refused: err.Error()})
		return
	}
	if err := rules.Allows(sent.Spec, adapter.EndpointOf()); err != nil {
		reply(w, http.StatusBadRequest, Answer{Refused: err.Error()})
		return
	}
	id, err := s.queued(sent.Spec, sent.Adapter)
	if err != nil {
		reply(w, http.StatusInternalServerError, Answer{Refused: err.Error()})
		return
	}
	reply(w, http.StatusCreated, Answer{ID: id})
}

// queued writes the job directory and takes it into the index, so a sent job
// leaves on disk what `haas new` leaves and the queue picks it up the same way.
func (s *Server) queued(spec job.Spec, carried *harness.Adapter) (string, error) {
	id := s.index.Free(spec.Brief, time.Now())
	dir := job.Dir(filepath.Join(s.index.Root(), id))
	if err := dir.Make(); err != nil {
		return "", err
	}
	if err := spec.Write(dir); err != nil {
		return "", err
	}
	if err := keep(dir, spec.Harness, carried); err != nil {
		return "", err
	}
	record := jobs.Record{
		ID: id, State: jobs.Queued, Brief: spec.Brief, Repo: spec.Repo,
		Harness: spec.Harness, Model: spec.Model, Asked: sentOverHTTP,
		Queued: jobs.Now(),
	}
	if err := s.index.Add(record); err != nil {
		return "", err
	}
	s.queue.Wake()
	return id, nil
}

// keep puts the adapter that arrived with the job where a carried adapter lives,
// so the job travels on complete. A job that brought none takes this machine's,
// the way a job made here does.
func keep(dir job.Dir, name string, carried *harness.Adapter) error {
	if carried == nil {
		return harness.Keep(dir, name)
	}
	text, err := json.MarshalIndent(carried, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(harness.Carried(dir), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(harness.Carried(dir), name+".json"), append(text, '\n'), 0o644)
}

// runnable is the adapter the job will run under: the one it carried, or the one
// this machine already has. A harness nobody here has heard of is the refusal
// only a sent job meets, because the directory crossed a machine.
func runnable(spec job.Spec, carried *harness.Adapter) (harness.Adapter, error) {
	if !plainName(spec.Harness) {
		return harness.Adapter{}, fmt.Errorf("a harness is named in letters, digits and dashes, and this job names %q", spec.Harness)
	}
	if carried != nil {
		if carried.Program == "" {
			return harness.Adapter{}, fmt.Errorf("the adapter carried for %s names no program to run", spec.Harness)
		}
		return *carried, nil
	}
	adapter, err := harness.Load("", spec.Harness)
	if err != nil {
		return harness.Adapter{}, fmt.Errorf("%w here, and the job carries no adapter for it", err)
	}
	return adapter, nil
}

// missing says what a runner cannot do without, and which of them this job has
// none of. Naming them is the difference between fixing a job and guessing at it.
func missing(spec job.Spec) string {
	var absent []string
	if strings.TrimSpace(spec.Repo) == "" {
		absent = append(absent, "a repository")
	}
	if strings.TrimSpace(spec.Brief) == "" {
		absent = append(absent, "a brief")
	}
	if strings.TrimSpace(strings.Join(spec.Gate, "")) == "" {
		absent = append(absent, "a gate")
	}
	switch len(absent) {
	case 0:
		return ""
	case 1:
		return "this job needs " + absent[0]
	}
	last := len(absent) - 1
	return "this job needs " + strings.Join(absent[:last], ", ") + " and " + absent[last]
}

// plainName keeps a harness name a name. It becomes a path inside the job
// directory, and one that arrived over HTTP is not trusted to stay there.
func plainName(name string) bool {
	if name == "" {
		return false
	}
	for _, letter := range name {
		if !unicode.IsLetter(letter) && !unicode.IsDigit(letter) && letter != '-' && letter != '_' {
			return false
		}
	}
	return true
}

// reply writes one JSON object and the status before it, so a script reads the
// reason as well as the code.
func reply(w http.ResponseWriter, status int, answer Answer) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(answer)
}
