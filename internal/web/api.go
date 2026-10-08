package web

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/getlatentic/haas/internal/quota"
	"github.com/getlatentic/haas/internal/repoaccess"
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
	key, err := requestID(r)
	if err != nil {
		reply(w, http.StatusBadRequest, Answer{Refused: err.Error()})
		return
	}
	if key != "" {
		s.keyed.Lock()
		defer s.keyed.Unlock()
		if already, found := s.index.FindKey(callerOf(r), key); found {
			reply(w, http.StatusOK, Answer{ID: already.ID})
			return
		}
	}
	if short := missing(sent.Spec); short != "" {
		reply(w, http.StatusBadRequest, Answer{Refused: short})
		return
	}
	if err := repoaccess.Acceptable(sent.Spec.Repo, s.localRepos); err != nil {
		reply(w, http.StatusBadRequest, Answer{Refused: err.Error()})
		return
	}
	if err := repoaccess.AcceptableRef(sent.Spec.Ref); err != nil {
		reply(w, http.StatusBadRequest, Answer{Refused: err.Error()})
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
	if err := reachable(r.Context(), sent.Spec.Repo); err != nil {
		reply(w, http.StatusBadRequest, Answer{Refused: err.Error()})
		return
	}
	id, err := s.queued(sent.Spec, sent.Adapter, callerOf(r), key)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, quota.ErrExceeded) {
			status = http.StatusTooManyRequests
		}
		reply(w, status, Answer{Refused: err.Error()})
		return
	}
	reply(w, http.StatusCreated, Answer{ID: id})
}

// reachable refuses by name a repository this deployment's GitHub App is not
// installed on, so whoever sent the job hears it now and not from a job that
// failed later.
func reachable(ctx context.Context, repo string) error {
	forge, err := repoaccess.FromEnv()
	if err != nil {
		return err
	}
	return forge.Check(ctx, repo)
}

// queued writes the job directory and takes it into the index, so a sent job
// leaves on disk what `haas new` leaves and the queue picks it up the same way.
func (s *Server) queued(spec job.Spec, carried *harness.Adapter, asked, key string) (string, error) {
	id, err := s.index.Reserve(spec.Brief, time.Now())
	if err != nil {
		return "", err
	}
	dir := job.Dir(filepath.Join(s.index.Root(), id))
	if err := dir.Make(); err != nil {
		return "", err
	}
	if err := s.equip(&spec, dir, asked); err != nil {
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
		Harness: spec.Harness, Model: spec.Model, Asked: asked, Key: key, Team: spec.Team,
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

// mostKey is the longest request id taken, which is more than any generated one needs.
const mostKey = 128

// requestID is the id a sender gave a send, in the Idempotency-Key header, or none. A send made again with the same id is
// the same send. The id is printable text and short, so it can be kept in a record and read in a log.
func requestID(r *http.Request) (string, error) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) > mostKey {
		return "", fmt.Errorf("an Idempotency-Key is at most %d characters", mostKey)
	}
	for _, c := range key {
		if c < 0x21 || c > 0x7e {
			return "", errors.New("an Idempotency-Key is printable characters with no spaces")
		}
	}
	return key, nil
}
