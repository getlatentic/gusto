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

func (s *Server) recordsPage(w http.ResponseWriter, r *http.Request) {
	var tabs []tab
	if s.hasChanges() {
		tabs = append(tabs, tab{Key: "activity", Label: "Activity"})
	}
	tabs = append(tabs, tab{Key: "checks", Label: "Task checks"})
	if s.hasRetention() {
		tabs = append(tabs, tab{Key: "keeping", Label: "Keeping"})
	}
	tabs, chosen := chooseTab(r, recordsPath, tabs)
	checks := s.checkRuns()
	health := s.recordHealth(checks)
	page := map[string]any{
		"Title": productName + " — records", "Tabs": tabs, "Tab": chosen, "Refused": r.URL.Query().Get("refused"),
		"Health": health, "Logged": s.hasChanges(),
	}
	switch chosen {
	case "activity":
		page["Rows"] = s.recentChanges()
	case "checks":
		page["Checks"] = checks
	case "keeping":
		if err := s.keepingInto(page); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	s.drawConsole(w, "admin-records.html", "records", page)
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
		// What the front door reported is taken into the log first, so the count includes it.
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
