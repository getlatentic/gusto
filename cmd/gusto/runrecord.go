package main

import (
	"path/filepath"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

// runRecord is who and what a run is for, from the server's index and rooms, for its signed record.
func runRecord(index *tasks.Index, instances *instance.Store, dir task.Dir, spec task.Spec) record.Run {
	run := record.Run{Room: spec.Session, Agent: spec.Agent}
	if entry, found := index.Get(filepath.Base(dir.Path())); found {
		run.Asked, run.PaidBy = entry.Asked, entry.Payer()
	}
	if held, found := instances.Get(spec.Session); found && spec.Agent != "" {
		if agent, found := held.AgentByID(spec.Agent); found {
			run.AgentName, run.AllowanceUSD = agent.Name, agent.AllowanceUSD
		}
	}
	return run
}
