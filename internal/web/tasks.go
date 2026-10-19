package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/jobs"
)

// runTask is work done on an instance's own computer: a brief, a gate, and the
// folder the instance already has. It is the one way a page runs a gated task,
// so it takes everything a job can be asked to carry: how sensitive the code
// is, the team its spend counts against, and a cap on what its calls may cost.
func (s *Server) runTask(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	where := "/instances/" + held.ID
	spec, err := taskOn(held, r)
	if err != nil {
		s.turnAway(w, r, where, err.Error())
		return
	}
	id, err := s.index.Reserve(spec.Brief, time.Now())
	if err != nil {
		s.turnAway(w, r, where, err.Error())
		return
	}
	record := jobs.Record{
		ID: id, State: jobs.Queued,
		Brief: spec.Brief, Repo: held.Repo, Harness: held.Harness, Model: held.Model,
		Instance: held.ID, Asked: s.keeper.who(r), Team: spec.Team, Queued: jobs.Now(),
	}
	if err := s.place(spec, record); err != nil {
		s.turnAway(w, r, where, err.Error())
		return
	}
	s.queue.Wake()
	http.Redirect(w, r, "/jobs/"+record.ID, http.StatusSeeOther)
}

// taskOn is the spec the task form describes, on the instance it was posted to.
// Anything the runner would refuse to read is refused here instead, so a job
// that cannot run is never written.
func taskOn(held instance.Instance, r *http.Request) (job.Spec, error) {
	spec := job.Spec{
		Brief:       strings.TrimSpace(r.FormValue("brief")),
		Gate:        job.Words(r.FormValue("gate")),
		Harness:     held.Harness,
		Model:       held.Model,
		Repo:        held.Repo,
		Session:     held.ID,
		Sensitivity: sensitivity(r.FormValue("sensitivity")),
		Team:        strings.TrimSpace(r.FormValue("team")),
		Setup:       setupOf(held),
	}
	if held.Sensitivity == string(job.Restricted) {
		spec.Sensitivity = job.Restricted
	}
	if spec.Brief == "" || len(spec.Gate) == 0 {
		return job.Spec{}, errors.New("a task needs something to do and a command that says it worked")
	}
	if !spec.Sensitivity.Known() {
		return job.Spec{}, fmt.Errorf("%q is not a sensitivity this server knows", spec.Sensitivity)
	}
	capped, err := dollars(r.FormValue("usd"))
	if err != nil {
		return job.Spec{}, err
	}
	spec.Budget.USD = capped
	return spec, spec.Budget.Check()
}

// sensitivity is the label picked, or internal when none was, which is what a
// job that names none is read as anyway.
func sensitivity(picked string) job.Sensitivity {
	if picked == "" {
		return job.Internal
	}
	return job.Sensitivity(picked)
}

// dollars is a cap a person typed. Nothing typed is no cap, which is not the
// same as a cap of nothing.
func dollars(typed string) (*float64, error) {
	typed = strings.TrimPrefix(strings.TrimSpace(typed), "$")
	if typed == "" {
		return nil, nil
	}
	amount, err := strconv.ParseFloat(typed, 64)
	if err != nil {
		return nil, fmt.Errorf("%q is not an amount of dollars", typed)
	}
	return &amount, nil
}

// tasks is the work done on an instance that was not said to it: a job that
// named this instance and no thread of it.
func (s *Server) tasks(id string) []jobs.Record {
	var done []jobs.Record
	for _, record := range s.index.List() {
		if record.Instance == id && record.Thread == "" {
			done = append(done, record)
		}
	}
	return done
}
