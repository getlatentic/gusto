package web

import (
	"fmt"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
)

// A turn that has not started says so, and says what holds it. Working is for a turn an agent is on.

// holdingOfLatest says what a one-agent conversation's running turn waits for, or nothing when it has started or there is none.
func (s *Server) holdingOfLatest(held instance.Instance, id string) string {
	if id == "" || held.Room {
		return ""
	}
	return s.waiting(id)
}

// holdingOf says what a turn that has not started waits for, in the words a room shows, or nothing for a turn that has started
// or is over. A room's agent takes one message at a time, so a turn behind its own agent's names that agent.
func (s *Server) holdingOf(held instance.Instance, record jobs.Record) string {
	wait, queued := s.index.WaitOf(record.ID)
	if !queued {
		return ""
	}
	if wait.Reason == jobs.AgentBusy {
		if agent, found := held.AgentByID(record.Agent); found {
			return fmt.Sprintf("Waiting for %s to finish the message before this one. Number %d in line.", agent.Name, wait.Position)
		}
	}
	return waitWords(wait)
}
