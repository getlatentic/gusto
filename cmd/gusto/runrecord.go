package main

import (
	"path/filepath"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/pkg/job"
)

// runRecord is who and what a run is for, from the server's index and rooms, for its signed record.
func runRecord(index *jobs.Index, instances *instance.Store, dir job.Dir, spec job.Spec) evidence.Run {
	run := evidence.Run{Room: spec.Session, Agent: spec.Agent}
	if record, found := index.Get(filepath.Base(dir.Path())); found {
		run.Asked, run.PaidBy = record.Asked, record.Payer()
	}
	if held, found := instances.Get(spec.Session); found && spec.Agent != "" {
		if agent, found := held.AgentByID(spec.Agent); found {
			run.AgentName, run.AllowanceUSD = agent.Name, agent.AllowanceUSD
		}
	}
	return run
}
