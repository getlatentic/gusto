package web

import (
	"fmt"
	"net/http"
)

// recordsPath is the console's page of the record: what happened, whether the finished runs still check out, and how long work is kept.
const recordsPath = "/admin/records"

const (
	activityTab = recordsPath + "?tab=activity"
	checksTab   = recordsPath + "?tab=checks"
	keepingTab  = recordsPath + "?tab=keeping"
)

// recordsTabs are the views of Records this server has: the activity log where it keeps one, the task checks, and keeping where
// it removes work.
func (s *Server) recordsTabs() []tab {
	var tabs []tab
	if s.hasChanges() {
		tabs = append(tabs, tab{Key: "activity", Label: "Activity"})
	}
	tabs = append(tabs, tab{Key: "checks", Label: "Task checks"})
	if s.hasRetention() {
		tabs = append(tabs, tab{Key: "keeping", Label: "Keeping"})
	}
	return tabs
}

// recordsData is the record's health on every tab, and the chosen tab's own part.
func (s *Server) recordsData(r *http.Request) (map[string]any, error) {
	tabs, chosen := chooseTab(r, recordsPath, s.recordsTabs())
	checks := s.checkRuns()
	page := map[string]any{
		"Tabs": tabs, "Tab": chosen, "Refused": s.refusal(r), "Health": s.recordHealth(checks), "Logged": s.hasChanges(),
	}
	switch chosen {
	case "activity":
		page["Activity"] = s.activityOf(r)
	case "checks":
		page["Checks"] = checks
	case "keeping":
		keeping, err := s.keepingOf()
		if err != nil {
			return nil, err
		}
		page["Keeping"] = keeping
	}
	return page, nil
}

// activityOf is the newest events, with the server starting left out unless the address asks for it.
func (s *Server) activityOf(r *http.Request) map[string]any {
	withServer := r.URL.Query().Has("server")
	rows, leftOut := s.recentChanges(withServer)
	return map[string]any{"Rows": rows, "LeftOut": leftOut, "WithServer": withServer}
}

// health is the record in one line: whether the activity log's chain holds and is signed, and whether the newest finished runs
// still match what was recorded when they finished.
type health struct {
	Logged, Broken bool
	Log, Runs      string
	// LogBad and RunsBad say which half is the trouble, so the page marks only that half.
	LogBad, RunsBad bool
	Head            string
}

func (s *Server) recordHealth(checks runChecks) health {
	h := health{Logged: s.hasChanges(), Runs: runsSaid(checks), RunsBad: checks.Failing > 0}
	if h.Logged {
		// What gusto cloud reported is taken into the log first, so the count includes it.
		s.TakeInbox()
		report, broken := s.checkLog()
		h.Head = report.Head
		if broken != "" {
			h.Log, h.LogBad = "the activity log does not hold together: "+broken, true
		} else {
			h.Log = eventsSaid(report.Events, report.Signed)
		}
	}
	h.Broken = h.LogBad || h.RunsBad
	return h
}

func eventsSaid(events, signed int) string {
	said := fmt.Sprintf("%d %s", events, plural(events, "event"))
	switch {
	case events == 0:
		return "no events yet"
	case signed == events:
		return said + ", all signed"
	case signed == 0:
		return said + ", none signed"
	}
	return fmt.Sprintf("%s, %d signed", said, signed)
}

func runsSaid(checks runChecks) string {
	checked := checks.Checked()
	switch {
	case checked == 0:
		return "no task has finished yet"
	case checked == 1 && checks.Failing == 1:
		return "the last task does not check out"
	case checked == 1:
		return "the last task checks out"
	case checks.Failing == 1:
		return fmt.Sprintf("1 of the last %d tasks does not check out", checked)
	case checks.Failing > 1:
		return fmt.Sprintf("%d of the last %d tasks do not check out", checks.Failing, checked)
	}
	return fmt.Sprintf("the last %d tasks check out", checked)
}
