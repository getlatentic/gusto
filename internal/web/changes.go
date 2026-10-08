package web

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/haas/internal/changelog"
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
	s.record(changelog.Event{Who: who, Role: string(role), From: clientOf(r), Kind: kind, What: what})
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
	from := clientOf(r)
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
	from := clientOf(r)
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

// clientOf is where a request came from. The address a reverse proxy on this
// machine forwards is believed, and one from anywhere else is not, because
// anybody can write that header.
func clientOf(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if forwarded, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ","); strings.TrimSpace(forwarded) != "" {
			return strings.TrimSpace(forwarded)
		}
	}
	return host
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
	At   string
	Who  string
	Said string
	From string
}

func (s *Server) changesPage(w http.ResponseWriter, r *http.Request) {
	s.TakeInbox()
	page := map[string]any{"Title": productName + " — activity", "Shown": shownChanges}
	if file, err := os.Open(s.changes.Path()); err == nil {
		report, verr := changelog.Verify(file, s.keyring)
		file.Close()
		page["Events"], page["Head"] = report.Events, report.Head
		page["Signed"], page["Unsigned"] = report.Signed, report.Unsigned
		if verr != nil {
			page["Broken"] = verr.Error()
		}
	}
	page["Rows"] = s.recentChanges()
	s.drawConsole(w, "admin-activity.html", "activity", page)
}

// recentChanges is the newest events first.
func (s *Server) recentChanges() []row {
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
		at := e.At
		if when, err := time.Parse(time.RFC3339Nano, e.At); err == nil {
			at = when.UTC().Format("2 Jan 2006 15:04:05 UTC")
		}
		rows = append(rows, row{At: at, Who: whoOf(e), Said: saidOf(e), From: e.From})
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	if len(rows) > shownChanges {
		rows = rows[:shownChanges]
	}
	return rows
}

func whoOf(e changelog.Event) string {
	if e.Who == "" {
		return "nobody signed in"
	}
	return e.Who
}

// saidOf says an event in words a person reads.
func saidOf(e changelog.Event) string {
	w := e.What
	switch e.Kind {
	case changelog.SignedIn:
		return fmt.Sprintf("signed in as %s, with %s", or(e.Role, "no role"), or(w["how"], "an unknown method"))
	case changelog.Refused:
		said := "was refused with " + or(w["how"], "an unknown method") + ": " + or(w["why"], "no reason given")
		if w["suppressed"] != "" {
			said += " (" + w["suppressed"] + " more from here were not written)"
		}
		return said
	case changelog.SignedOut:
		return "signed out"
	case changelog.RoleChanged:
		return fmt.Sprintf("role changed from %s to %s", or(w["was"], "none"), or(w["now"], "none"))
	case changelog.SetupSaved:
		return fmt.Sprintf("%s skill set %s, with tools: %s", w["action"], w["setup"], or(w["servers"], "none"))
	case changelog.SkillWritten:
		return fmt.Sprintf("wrote the skill %s in %s", w["skill"], w["setup"])
	case changelog.SkillDropped:
		return fmt.Sprintf("removed the skill %s from %s", w["skill"], w["setup"])
	case changelog.TokenCreated:
		return fmt.Sprintf("made the API token %s, which may %s", w["name"], or(w["scopes"], "do nothing"))
	case changelog.TokenRevoked:
		return fmt.Sprintf("revoked the API token %s", w["name"])
	case changelog.HoldSet:
		return "kept " + heldThing(w) + " from being removed" + becauseOf(w["why"])
	case changelog.HoldReleased:
		return "let " + heldThing(w) + " be removed on schedule again"
	case changelog.Swept:
		return fmt.Sprintf("retention removed %s workspaces and %s records, freeing %s bytes%s",
			or(w["workspaces"], "0"), or(w["records"], "0"), or(w["freed"], "0"), recordsOf(w["jobs"]))
	case changelog.BudgetSet:
		return fmt.Sprintf("changed the monthly budget from %s to %s", or(w["was"], "none"), or(w["now"], "none"))
	case changelog.ProviderAdded:
		return fmt.Sprintf("added the model provider %s at %s, with its key %s", w["provider"], w["address"], or(w["key"], "kept"))
	case changelog.ProviderRetired:
		return fmt.Sprintf("retired the model provider %s", w["provider"])
	case changelog.KeyReplaced:
		return fmt.Sprintf("gave the model provider %s another key", w["provider"])
	case changelog.ModelAdded:
		return fmt.Sprintf("added the model %s, priced %s", w["model"], or(w["price"], "none"))
	case changelog.ProviderEnabled, changelog.ProviderDisabled:
		return fmt.Sprintf("turned the model provider %s %s", w["provider"], w["now"])
	case changelog.ModelEnabled, changelog.ModelDisabled:
		return fmt.Sprintf("turned the model %s %s", w["model"], w["now"])
	case changelog.RegistryRestored:
		return fmt.Sprintf("put the models back as they were at version %s, which undid: %s", w["version"], or(w["undid"], "nothing"))
	case changelog.IntegrationImported:
		return fmt.Sprintf("imported %s %s (%s)", w["integration"], or(w["label"], w["version"]), or(w["source"], "upload"))
	case changelog.IntegrationEnabled:
		return fmt.Sprintf("turned on %s, version %s, in place of %s%s", w["integration"], w["to"], or(w["from"], "none"), becauseOf(w["changes"]))
	case changelog.IntegrationDisabled:
		return fmt.Sprintf("turned off %s, which was version %s", w["integration"], or(w["from"], "none"))
	case changelog.IntegrationRestored:
		return fmt.Sprintf("put %s back to version %s, from %s", w["integration"], w["to"], or(w["from"], "none"))
	case changelog.IntegrationPrograms:
		return fmt.Sprintf("changed which version of %s may run its programs, from %s to %s", w["integration"], or(w["was"], "none"), or(w["allowed"], "none"))
	case changelog.IntegrationScoped:
		return fmt.Sprintf("changed who %s is on for, from %s to %s", w["integration"], or(w["was"], "everybody"), or(w["now"], "everybody"))
	case changelog.ModelRetired:
		return fmt.Sprintf("retired the model %s", w["model"])
	case changelog.PriceSet:
		return fmt.Sprintf("priced the model %s at %s", w["model"], or(w["price"], "none"))
	case changelog.PolicySet:
		return fmt.Sprintf("set what model providers may be: private addresses %s, keys from %s", or(w["private"], "no"), or(w["keys"], "a typed key only"))
	case changelog.AccessSet:
		if w["models"] == "" {
			return fmt.Sprintf("let %s use every model again", w["email"])
		}
		return fmt.Sprintf("limited %s to the models %s", w["email"], w["models"])
	case changelog.MemberInvited:
		return fmt.Sprintf("invited %s to join as a %s", w["email"], or(w["role"], "runner"))
	case changelog.MemberRole:
		return fmt.Sprintf("changed the role of %s from %s to %s", w["email"], or(w["was"], "none"), or(w["now"], "none"))
	case changelog.MemberRemoved:
		return fmt.Sprintf("removed %s, who was %s", w["email"], or(w["role"], "a member"))
	case changelog.ServerStarted:
		return "the server started"
	case changelog.SettingsChanged:
		return fmt.Sprintf("the setting %s changed from %q to %q", w["setting"], w["was"], w["now"])
	}
	return e.Kind
}

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func heldThing(w map[string]string) string {
	if w["job"] != "" {
		return "the job " + w["job"]
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
