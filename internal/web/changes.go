package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
)

// UseChanges gives the server the log it records what changes in. Without one,
// nothing is recorded and the pages that show it answer 404.
func (s *Server) UseChanges(log *changelog.Log) { s.changes = log }

// UseSettings records the settings that decide who may do what, as this server
// started with them, and every one that differs from the last start. Whoever
// changed them did it outside the pages, so the record says the server saw it.
func (s *Server) UseSettings(now map[string]string) {
	if s.changes == nil {
		return
	}
	before := s.changes.LastSettings()
	if len(before) > 0 {
		for _, key := range differing(before, now) {
			s.record(changelog.Event{Who: "the server", Kind: changelog.SettingsChanged,
				What: map[string]string{"setting": key, "was": before[key], "now": now[key]}})
		}
	}
	s.record(changelog.Event{Who: "the server", Kind: changelog.ServerStarted, What: now})
}

// NoteSetting records that the server read a setting again and found it changed, or refused what it found. Fingerprints stand for
// the contents, which are not kept.
func (s *Server) NoteSetting(setting, was, now, refused string) {
	if s.changes == nil {
		return
	}
	what := map[string]string{"setting": setting, "was": was, "now": now}
	if refused != "" {
		what["refused"] = refused
	}
	s.record(changelog.Event{Who: "the server", Kind: changelog.SettingsChanged, What: what})
}

func differing(was, now map[string]string) []string {
	var keys []string
	seen := map[string]bool{}
	for key := range was {
		seen[key] = true
	}
	for key := range now {
		seen[key] = true
	}
	for key := range seen {
		if was[key] != now[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// note records something the person making this request just did.
func (s *Server) note(r *http.Request, kind string, what map[string]string) {
	if s.changes == nil {
		return
	}
	who, role, _ := s.keeper.session(r)
	s.record(changelog.Event{Who: who, Role: string(role), From: s.clientOf(r), Kind: kind, What: what})
}

// record writes an event. A change has already happened by the time it is
// written, so a failure to write is said loudly and does not undo it.
func (s *Server) record(e changelog.Event) {
	if _, err := s.changes.Append(e); err != nil {
		slog.Error("a change could not be recorded", "kind", e.Kind, "error", err)
	}
}

// noteSignIn records somebody signing in, and a role that is not the one they
// last signed in with.
func (s *Server) noteSignIn(r *http.Request, who string, role Role, how string) {
	if s.changes == nil {
		return
	}
	from := s.clientOf(r)
	if was, known := s.changes.LastRole(who); known && was != string(role) {
		s.record(changelog.Event{Who: who, Role: string(role), From: from, Kind: changelog.RoleChanged,
			What: map[string]string{"was": was, "now": string(role)}})
	}
	s.record(changelog.Event{Who: who, Role: string(role), From: from, Kind: changelog.SignedIn, What: map[string]string{"how": how}})
}

// noteRefused records a sign-in that was turned away, and why. Anybody can cause
// one, so a source that causes more than a few a minute has the rest counted and
// not written, or the log would be a way to fill the disk.
func (s *Server) noteRefused(r *http.Request, who, how, why string) {
	if s.changes == nil {
		return
	}
	from := s.clientOf(r)
	what := map[string]string{"how": how, "why": why}
	allowed, suppressed := s.turnedAway.allow(from, time.Now())
	if !allowed {
		return
	}
	if suppressed > 0 {
		what["suppressed"] = fmt.Sprint(suppressed)
	}
	s.record(changelog.Event{Who: who, From: from, Kind: changelog.Refused, What: what})
}

// refusals counts sign-ins turned away, by where they came from.
type refusals struct {
	mu      sync.Mutex
	sources map[string]*window
}

type window struct {
	from       time.Time
	written    int
	suppressed int
}

const (
	refusalsPerMinute = 10
	refusalWindow     = time.Minute
)

// allow says whether this refusal is written, and how many from the same source
// were not since the last that was.
func (f *refusals) allow(from string, now time.Time) (bool, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sources == nil {
		f.sources = map[string]*window{}
	}
	for key, w := range f.sources {
		if now.Sub(w.from) > 10*refusalWindow {
			delete(f.sources, key)
		}
	}
	w := f.sources[from]
	if w == nil {
		w = &window{from: now}
		f.sources[from] = w
	}
	if now.Sub(w.from) > refusalWindow {
		w.from, w.written = now, 0
	}
	if w.written >= refusalsPerMinute {
		w.suppressed++
		return false, 0
	}
	w.written++
	carried := w.suppressed
	w.suppressed = 0
	return true, carried
}

// changesFile is the whole log, as the file holds it, for an admin to export
// and for a verifier to check.
func (s *Server) changesFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="changes.jsonl"`)
	http.ServeFile(w, r, s.changes.Path())
}

// changesScript is the same file for a script with the shared password.
func (s *Server) changesScript(w http.ResponseWriter, r *http.Request) {
	if s.changes == nil {
		http.NotFound(w, r)
		return
	}
	s.changesFile(w, r)
}

// shownChanges is how many of the newest events the page lists.
const shownChanges = 200

// row is one event as the page says it.
type row struct {
	At    string
	Full  string
	Who   string
	Said  string
	From  string
	Times int
	Since string
	// Kind is the event's kind, which the page filters on.
	Kind string
}

// checkLog reads the whole activity log and says whether its chain holds, how much of it is signed, and its head.
func (s *Server) checkLog() (changelog.Report, string) {
	file, err := os.Open(s.changes.Path())
	if err != nil {
		return changelog.Report{}, ""
	}
	defer file.Close()
	report, err := changelog.Verify(file, s.keyring)
	if err != nil {
		return report, err.Error()
	}
	return report, ""
}

// recentChanges is the newest events first. People's actions are what the page is for, so a server starting, which every deploy
// does, is left out unless asked for, and how many were left out is said.
func (s *Server) recentChanges(withServerStarts bool) (rows []row, leftOut int) {
	for _, each := range s.changeRows() {
		if each.Kind == changelog.ServerStarted && !withServerStarts {
			leftOut++
			continue
		}
		rows = append(rows, each)
	}
	rows = foldRepeats(rows)
	if len(rows) > shownChanges {
		rows = rows[:shownChanges]
	}
	return rows, leftOut
}

// changeRows is every event in the log as the page says it, newest first.
func (s *Server) changeRows() []row {
	file, err := os.Open(s.changes.Path())
	if err != nil {
		return nil
	}
	defer file.Close()
	var rows []row
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for scanner.Scan() {
		var e changelog.Event
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			continue
		}
		at, full := e.At, e.At
		if when, err := time.Parse(time.RFC3339Nano, e.At); err == nil {
			at, full = shortWhen(when.UTC(), time.Now().UTC()), when.UTC().Format("2 Jan 2006 15:04:05 UTC")
		}
		rows = append(rows, row{At: at, Full: full, Who: whoOf(e), Said: saidOf(e), From: e.From, Times: 1, Kind: e.Kind})
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows
}

// shortWhen is a time as short as it can be and still be clear: the clock for today, the day and the clock for this year, else with the year.
func shortWhen(when, now time.Time) string {
	switch {
	case when.Format("2006-01-02") == now.Format("2006-01-02"):
		return "Today " + when.Format("15:04")
	case when.Year() == now.Year():
		return when.Format("2 Jan 15:04")
	}
	return when.Format("2 Jan 2006 15:04")
}

// foldRepeats joins runs of the same thing said by the same person from the same place into one row that says how many times, so that a
// server that restarted twelve times is one line and not twelve. The row keeps the newest time and says when the run began.
func foldRepeats(rows []row) []row {
	var out []row
	for _, each := range rows {
		last := len(out) - 1
		if last >= 0 && out[last].Who == each.Who && out[last].Said == each.Said && out[last].From == each.From {
			out[last].Times++
			out[last].Since = each.Full
			continue
		}
		out = append(out, each)
	}
	return out
}

func whoOf(e changelog.Event) string {
	if e.Who == "" {
		return "nobody signed in"
	}
	return e.Who
}

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func heldThing(w map[string]string) string {
	if w["job"] != "" {
		return "the task " + w["job"]
	}
	return "everything asked for by " + w["person"]
}

func becauseOf(why string) string {
	if why == "" {
		return ""
	}
	return ": " + why
}

func recordsOf(ids string) string {
	if ids == "" {
		return ""
	}
	return " (records removed: " + ids + ")"
}

// agentAppWords is a person's choice for an agent and one app of theirs. An entry from before choices were made app by app names
// no app, and was on or off for them all.
func agentAppWords(w map[string]string) string {
	switch {
	case w["connector"] == "" && w["now"] == "on":
		return fmt.Sprintf("let %s use their connected apps, for what they ask it", w["agent"])
	case w["connector"] == "":
		return fmt.Sprintf("stopped %s using their connected apps", w["agent"])
	case w["now"] == "allow":
		return fmt.Sprintf("let %s use their %s, for what they ask it", w["agent"], w["connector"])
	case w["now"] == "deny":
		return fmt.Sprintf("kept %s from their %s", w["agent"], w["connector"])
	}
	return fmt.Sprintf("left %s to follow their default for %s", w["agent"], w["connector"])
}
