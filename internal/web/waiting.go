package web

import (
	"fmt"

	"github.com/getlatentic/haas/internal/jobs"
)

// waiting says in words where a queued job stands and what it waits for, or
// nothing for a job that is not queued.
func (s *Server) waiting(id string) string {
	wait, queued := s.index.WaitOf(id)
	if !queued {
		return ""
	}
	return waitWords(wait)
}

func waitWords(wait jobs.Wait) string {
	place := fmt.Sprintf("Number %d in line.", wait.Position)
	switch wait.Reason {
	case jobs.AgentBusy:
		return "Waiting for this agent to finish its last message. " + place
	case jobs.PersonAtLimit:
		return fmt.Sprintf("%s already has %d running, the most one person may run at once. It starts when one ends. %s",
			askerOf(wait.Who), wait.Running, place)
	case jobs.TeamAtLimit:
		return fmt.Sprintf("The team %s already has %d running, the most a team may run at once. It starts when one ends. %s",
			wait.Who, wait.Running, place)
	case jobs.PoolFull:
		return fmt.Sprintf("Every runner is busy: %d of %d. It starts when one is free. %s", wait.Running, wait.Limit, place)
	}
	return "Starting now."
}
