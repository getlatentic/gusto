package web

import (
	"github.com/getlatentic/haas/internal/computer"
)

// UseComputers hands the console the record of the computers people in this organisation
// have. Without it the Overview has no computers to list.
func (s *Server) UseComputers(registry *computer.Registry) { s.computers = registry }

// computerRow is one person's computer as the Overview says it.
type computerRow struct {
	Person     string
	State      string
	Note       string
	LastActive int64
}

// addComputers lists the computers the organisation has and what state each is in.
func (s *Server) addComputers(page map[string]any) {
	if s.computers == nil {
		return
	}
	all, err := s.computers.All()
	if err != nil {
		page["ComputersUnreadable"] = err.Error()
		return
	}
	rows := make([]computerRow, 0, len(all))
	for _, each := range all {
		row := computerRow{Person: each.User, State: string(each.State), Note: each.Note}
		if !each.LastActive.IsZero() {
			row.LastActive = each.LastActive.Unix()
		}
		rows = append(rows, row)
	}
	page["Computers"], page["KeepsComputers"] = rows, true
}

// appRow is one shared web app as the Overview says it.
type appRow struct {
	Owner   string
	Agent   string
	State   string
	Note    string
	Started int64
}

// addApps lists the web apps people have shared from their agents.
func (s *Server) addApps(page map[string]any) {
	if s.apps == nil {
		return
	}
	all, err := s.apps.All()
	if err != nil {
		page["AppsUnreadable"] = err.Error()
		return
	}
	rows := make([]appRow, 0, len(all))
	for _, each := range all {
		row := appRow{Owner: each.Owner, State: string(each.State), Note: each.Note, Started: each.Started, Agent: "an agent that was removed"}
		if held, found := s.instances.Get(each.Instance); found {
			row.Agent = s.agentOf(held).Name
		}
		rows = append(rows, row)
	}
	page["Apps"], page["SharesApps"] = rows, true
}
