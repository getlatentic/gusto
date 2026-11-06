package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/quota"
	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/pkg/spend"
)

// openRecords opens the tasks and the instances. An organization's gusto cloud writes what the organization may run, and this
// server reads it as it goes; one that cannot read it does not start.
func (s *server) openRecords() error {
	index, err := jobs.Open(setting("GUSTO_JOBS", "jobs"))
	if err != nil {
		return err
	}
	finishDecisions(index)
	if path := os.Getenv("GUSTO_QUOTA"); path != "" {
		s.limits = quota.Open(path)
		if _, err := s.limits.Current(); err != nil {
			return err
		}
		index.UseQuota(s.limits)
	}
	// The directory keeps the name it was written under: what is in it is the same thing, read now as an instance with
	// threads rather than as one conversation.
	instances, err := instance.Open(setting("GUSTO_SESSIONS", "sessions"))
	if err != nil {
		return err
	}
	s.index, s.instances = index, instances
	return nil
}

// openWayOut opens the proxy turns reach their models through, with the connectors a turn's tools use and the credit the
// organization's cloud gives it.
func (s *server) openWayOut() error {
	way, err := openWayOut(s.index.Root())
	if err != nil {
		return err
	}
	if s.connect, err = openConnectors(); err != nil {
		return err
	}
	if s.connect != nil {
		way.gateways.tokens = s.connect
	}
	if s.limits != nil {
		used := filepath.Join(filepath.Dir(os.Getenv("GUSTO_QUOTA")), "credit-used.json")
		if err := way.ledger.UseCredit(creditOf(s.limits), used); err != nil {
			return err
		}
	}
	s.way = way
	return nil
}

// startTurns makes the boxes turns run in, starts the proxy where they reach it, and starts the queue that runs the turns.
// Pages an agent built are shared wherever it works; an app with a command of its own needs the agent's computer.
func (s *server) startTurns() error {
	shared, err := apps.Open(setting("GUSTO_APPS", "apps"))
	if err != nil {
		return err
	}
	boxes, err := openBackend(shared, s.way.proxyPort())
	if err != nil {
		return err
	}
	if err := boxes.Check(); err != nil {
		return err
	}
	stopProxy, err := s.way.Listen(boxes.Reach())
	if err != nil {
		return err
	}
	s.started(stopProxy)
	s.shared, s.boxes = shared, boxes
	s.index.SetLimits(jobs.Limits{Person: number("GUSTO_LIMIT_PERSON", 0), Team: number("GUSTO_LIMIT_TEAM", 0)})
	s.rooms = newRoomRuns(s.instances, func(dir job.Dir) string { return personWhoAsked(s.index, dir) })
	s.tellWayOut()
	// Started before any turn: a backend keeps up what its turns use, such as its image or people's computers.
	stopBoxes, err := boxes.Start(context.Background())
	if err != nil {
		return err
	}
	s.started(stopBoxes)
	s.queue = jobs.StartStoppable(s.index, turns(boxes, s.way, s.instances, s.index, s.rooms), number("GUSTO_JOB_CONCURRENCY", 1))
	s.started(s.queue.Stop)
	return nil
}

// tellWayOut says how the proxy finds who a run is for: the task's record and its room, which the index and instances hold.
func (s *server) tellWayOut() {
	s.way.UseRunRecords(func(dir job.Dir, spec job.Spec) evidence.Run { return runRecord(s.index, s.instances, dir, spec) })
	s.way.UseAskers(func(id string) string {
		if record, found := s.index.Get(id); found {
			return record.Asked
		}
		return ""
	})
}

// creditOf is the credit an organization's gusto cloud gives it, read from its quota as the quota changes. A quota that cannot be
// read is a credit of nothing, so a server that lost its limits spends nothing of the operator's.
func creditOf(limits *quota.File) spend.CreditLimit {
	return func() (float64, bool) {
		current, err := limits.Current()
		if err != nil {
			return 0, true
		}
		return current.CreditUSD, current.CreditUSD > 0
	}
}
