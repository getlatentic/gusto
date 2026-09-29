package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/retention"
)

// lastSweepFile is where the newest sweep's report is kept, so the page can
// show it after a restart.
const lastSweepFile = "last.json"

// usageFor is how long a measure of the disk is kept. It walks every job's
// directory, and a page that is opened often should not do that each time.
const usageFor = time.Minute

// listedRecords is how many removed records one change-record line names. The
// last sweep's report, kept beside the policy, names them all.
const listedRecords = 200

type retained struct {
	dir    string
	policy retention.Policy
	holds  *retention.Holds
	// sweeping makes one sweep at a time, whether the schedule or an admin asked.
	sweeping sync.Mutex
	mu       sync.Mutex
	usage    retention.Usage
	measured time.Time
}

// UseRetention removes finished work on the policy's schedule, and gives an
// admin the page that shows the disk, the holds and the last sweep. Without it
// nothing is removed and the pages answer 404.
func (s *Server) UseRetention(dir string, policy retention.Policy) {
	s.retention = &retained{dir: dir, policy: policy, holds: retention.OpenHolds(dir)}
}

// RunRetention sweeps every interval until ctx ends. A policy that removes
// nothing sweeps nothing.
func (s *Server) RunRetention(ctx context.Context, every time.Duration) {
	if s.retention == nil || !s.retention.policy.Any() {
		return
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		s.Sweep(time.Now())
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Sweep removes what the policy says is due, records it, and keeps the report.
func (s *Server) Sweep(now time.Time) retention.Report {
	r := s.retention
	r.sweeping.Lock()
	defer r.sweeping.Unlock()
	held, err := r.holds.Read()
	if err != nil {
		// A sweep that cannot read its holds must not guess that there are none.
		slog.Error("retention could not read its holds, so it removed nothing", "error", err)
		return retention.Report{At: now.Unix()}
	}
	report := retention.Sweep(s.index, s.index.Root(), r.policy, held, now, false)
	if len(report.Actions) > 0 {
		s.recordSweep(report)
	}
	r.keep(report)
	return report
}

func (r *retained) keep(report retention.Report) {
	text, err := json.MarshalIndent(report, "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(r.dir, lastSweepFile), append(text, '\n'), 0o640)
	}
	if err != nil {
		slog.Error("the last sweep could not be kept", "error", err)
	}
}

func (r *retained) last() (retention.Report, bool) {
	text, err := os.ReadFile(filepath.Join(r.dir, lastSweepFile))
	if err != nil {
		return retention.Report{}, false
	}
	var report retention.Report
	return report, json.Unmarshal(text, &report) == nil
}

// recordSweep writes what a sweep removed to the change record: removing a
// signed record is a change an auditor asks about.
func (s *Server) recordSweep(report retention.Report) {
	if s.changes == nil {
		return
	}
	var records []string
	for _, action := range report.Actions {
		if action.Kind == retention.Record && action.Failed == "" {
			records = append(records, action.Job)
		}
	}
	listed := records
	if len(listed) > listedRecords {
		listed = append(listed[:listedRecords:listedRecords], fmt.Sprintf("and %d more", len(records)-listedRecords))
	}
	s.record(changelog.Event{Who: "the server", Kind: changelog.Swept, What: map[string]string{
		"workspaces": fmt.Sprint(report.Removed(retention.Workspace)),
		"records":    fmt.Sprint(len(records)),
		"jobs":       strings.Join(listed, " "),
		"freed":      fmt.Sprint(report.Freed),
	}})
}

// measured is the disk the jobs take, kept for a minute.
func (r *retained) measure(s *Server) retention.Usage {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.measured) > usageFor {
		r.usage, r.measured = retention.Measure(s.index.Root(), s.index.List()), time.Now()
	}
	return r.usage
}

// retentionAdmin answers 404 to anybody who is not an admin, and to every
// request when the server removes nothing.
func (s *Server) retentionAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.retention == nil || !s.admin(r) {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

func (s *Server) retentionPage(w http.ResponseWriter, r *http.Request) {
	held, err := s.retention.holds.Read()
	if err != nil {
		http.Error(w, "the holds could not be read: "+err.Error(), http.StatusInternalServerError)
		return
	}
	used := s.retention.measure(s)
	page := map[string]any{
		"Title": "haas — retention", "Nav": "retention", "Admin": true,
		"Refused": r.URL.Query().Get("refused"),
		"Used":    size(used.Bytes), "Workspaces": size(used.Workspaces), "Jobs": used.Jobs,
		"Policy": s.retention.policy.Rows(), "Removes": s.retention.policy.Any(),
		"HeldJobs": held.Jobs, "HeldPeople": held.People,
	}
	if used.Oldest != nil {
		page["Oldest"], page["OldestAt"] = used.Oldest.ID, time.Unix(used.Oldest.Ended, 0).UTC().Format("2 Jan 2006")
	}
	if last, found := s.retention.last(); found {
		page["Last"] = last
		page["LastAt"] = time.Unix(last.At, 0).UTC().Format("2 Jan 2006 15:04 UTC")
		page["LastFreed"] = size(last.Freed)
	}
	upcoming := retention.Sweep(s.index, s.index.Root(), s.retention.policy, held, time.Now(), true)
	page["Upcoming"], page["UpcomingFreed"] = upcoming.Actions, size(sumBytes(upcoming))
	s.render(w, "retention.html", page)
}

func sumBytes(report retention.Report) int64 {
	var total int64
	for _, action := range report.Actions {
		total += action.Bytes
	}
	return total
}

// size is a number of bytes as a person reads it.
func size(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value, suffix := float64(bytes), ""
	for _, next := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		suffix = next
		if value < unit {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}

// sweepNow is an admin asking for the sweep the schedule would make.
func (s *Server) sweepNow(w http.ResponseWriter, r *http.Request) {
	s.Sweep(time.Now())
	http.Redirect(w, r, "/retention", http.StatusSeeOther)
}

// holdJob keeps one job whole, and releaseJob ends that.
func (s *Server) holdJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, found := s.index.Get(id); !found {
		http.NotFound(w, r)
		return
	}
	why := strings.TrimSpace(r.FormValue("why"))
	if err := s.retention.holds.HoldJob(id, s.keeper.who(r), why); err != nil {
		s.turnAway(w, r, "/jobs/"+id, err.Error())
		return
	}
	s.note(r, changelog.HoldSet, map[string]string{"job": id, "why": why})
	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}

func (s *Server) releaseJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.retention.holds.ReleaseJob(id); err != nil {
		s.turnAway(w, r, "/jobs/"+id, err.Error())
		return
	}
	s.note(r, changelog.HoldReleased, map[string]string{"job": id})
	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}

// holdPerson keeps everything a person asked for whole, and releasePerson ends it.
func (s *Server) holdPerson(w http.ResponseWriter, r *http.Request) {
	who, why := strings.TrimSpace(r.FormValue("who")), strings.TrimSpace(r.FormValue("why"))
	if who == "" {
		s.turnAway(w, r, "/retention", "name the person whose work to keep")
		return
	}
	if err := s.retention.holds.HoldPerson(who, s.keeper.who(r), why); err != nil {
		s.turnAway(w, r, "/retention", err.Error())
		return
	}
	s.note(r, changelog.HoldSet, map[string]string{"person": strings.ToLower(who), "why": why})
	http.Redirect(w, r, "/retention", http.StatusSeeOther)
}

func (s *Server) releasePerson(w http.ResponseWriter, r *http.Request) {
	who := strings.TrimSpace(r.FormValue("who"))
	if err := s.retention.holds.ReleasePerson(who); err != nil {
		s.turnAway(w, r, "/retention", err.Error())
		return
	}
	s.note(r, changelog.HoldReleased, map[string]string{"person": strings.ToLower(who)})
	http.Redirect(w, r, "/retention", http.StatusSeeOther)
}

// drawHold adds what an admin needs to keep this job from being removed, or to
// let it go, on a server that removes work.
func (s *Server) drawHold(page map[string]any, r *http.Request, id string) {
	if s.retention == nil || !s.admin(r) {
		return
	}
	page["Admin"], page["CanHold"] = true, true
	held, err := s.retention.holds.Read()
	if err != nil {
		return
	}
	if hold, on := held.Jobs[id]; on {
		page["Hold"] = hold
	}
}
