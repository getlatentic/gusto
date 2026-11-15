package web

import (
	"github.com/getlatentic/gusto/pkg/spend"
)

// otherWork is what spent without an agent: a task sent to the API, a matrix
// run, a smoke test.
const otherWork = "Other work"

// byAgent is the month's tasks added up under the agent each was said to.
func (s *Server) byAgent(tasks []spend.Line) []spend.Line {
	return plainly(tasks, func(id string) string {
		record, found := s.index.Get(id)
		if !found || record.Instance == "" {
			return otherWork
		}
		held, found := s.instances.Get(record.Instance)
		if !found {
			return otherWork
		}
		return s.agentOf(held).Name
	})
}

// anyNamed is a breakdown worth drawing: one where somebody named something.
func anyNamed(lines []share) []share {
	for _, line := range lines {
		if line.Name != "" {
			return lines
		}
	}
	return nil
}
