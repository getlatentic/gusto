package web

import (
	"encoding/json"
	"fmt"
	"github.com/getlatentic/gusto/internal/workspace"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/task"
)

var helpers = template.FuncMap{
	"modelName": modelName,
	// dict passes more than one value to a template defined in another, which
	// html/template has no literal for.
	"dict": func(pairs ...any) map[string]any {
		made := map[string]any{}
		for i := 0; i+1 < len(pairs); i += 2 {
			if key, ok := pairs[i].(string); ok {
				made[key] = pairs[i+1]
			}
		}
		return made
	},
	"when": func(unix int64) string {
		if unix == 0 {
			return ""
		}
		return time.Unix(unix, 0).Format("2 Jan 15:04")
	},
	// money is a cost a harness reported. It is a pointer because a cost of
	// nothing and no cost reported are different answers, and a page must not
	// show one as the other — and because a pointer handed to printf prints an
	// address rather than an amount.
	"money": func(usd *float64) string {
		if usd == nil {
			return ""
		}
		return fmt.Sprintf("$%.4f", *usd)
	},
	"tokens":  compact,
	"brain":   modelName,
	"hue":     fieldClass,
	"initial": initial,
	// split is a list text, such as "send tasks, read spending", as its items.
	"split": func(text, by string) []string { return strings.Split(text, by) },
	// modelOnly is a model without the credential that serves it, which is
	// what somebody choosing between models reads.
	"modelOnly": func(model string) string {
		if _, alone, found := strings.Cut(model, "/"); found {
			return alone
		}
		return model
	},
	// span is a number of seconds as a person says it.
	"size": measure,
	"span": func(seconds int64) string { return (time.Duration(seconds) * time.Second).String() },
	"has":  func(list []string, item string) bool { return slices.Contains(list, item) },
	"seconds": func(wall *float64) string {
		if wall == nil {
			return ""
		}
		return (time.Duration(*wall) * time.Second).String()
	},
	"markdown": markdown,
	"asset":    asset,
	"logo":     logo,
	"logoDark": logoDark,
	"base":     filepath.Base,
	"add":      func(a, b int64) int64 { return a + b },
	// usd is an amount of money as a headline shows it: cents once there are
	// dollars, and the fractions of a cent an agent's calls cost until then.
	"usd": headlineUSD,
	// ago is how long ago something happened, as a list read at a glance says
	// it, with the date itself once it is more than a day old.
	"ago": func(unix int64) string {
		if unix == 0 {
			return ""
		}
		since := time.Since(time.Unix(unix, 0))
		switch {
		case since < time.Minute:
			return "just now"
		case since < time.Hour:
			return fmt.Sprintf("%dm ago", int(since.Minutes()))
		case since < 24*time.Hour:
			return fmt.Sprintf("%dh ago", int(since.Hours()))
		}
		return time.Unix(unix, 0).Format("2 Jan")
	},
	// percent is a part of a whole, rounded, for a label.
	"percent": func(part, whole int64) string {
		if whole == 0 {
			return "0%"
		}
		return fmt.Sprintf("%.0f%%", 100*float64(part)/float64(whole))
	},
	"took": took,
}

// took is how long something ran from one unix second to another, or nothing when it has not started or ended.
func took(from, to int64) string {
	if from == 0 || to == 0 {
		return ""
	}
	return (time.Duration(to-from) * time.Second).String()
}

// headlineUSD is an amount of money as a headline shows it: cents once there are
// dollars or there is nothing, and the fractions of a cent an agent's calls cost until then.
func headlineUSD(amount float64) string {
	if amount >= 1 || amount == 0 {
		return fmt.Sprintf("$%.2f", amount)
	}
	return fmt.Sprintf("$%.4f", amount)
}

// loginPage is the sign-in page, for whoever is not signed in yet.
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if s.keeper.knows(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.publicPage(w, r)
}

// signInView is what the sign-in page offers. Wrong is a shared password that was not right, and Local a server on a person's own
// computer, which opens with a link and not a password. Identity is the provider to sign in with, and Provider that there is one,
// which in the desktop app is a browser to continue in. Shared is whether the shared password's form is open.
type signInView struct {
	Wrong, Local, Identity, Provider, Desktop, Shared bool
	Refused                                           string
}

// loginData is what the sign-in page offers. Behind gusto cloud the page is only the desktop app's, which continues in the
// person's browser, since gusto cloud is where people sign in.
func (s *Server) loginData(r *http.Request) (map[string]any, error) {
	query := r.URL.Query()
	view := signInView{Desktop: true, Provider: true}
	if s.front == nil {
		wrong := query.Get("wrong") != ""
		view = signInView{
			Wrong: wrong, Local: s.local != nil,
			Identity: s.identity.Configured(), Provider: s.identity.Configured(), Desktop: query.Has("desktop"),
			// With a provider the shared password is the way in when the provider is not, so it stays reachable and stops being the
			// second thing on the page. A wrong one keeps it open, or there is nowhere to try again.
			Shared: !s.identity.Configured() || wrong || query.Has("shared"),
		}
	}
	view.Refused = s.refusal(r)
	return map[string]any{"SignIn": view}, nil
}

func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	from := s.clientOf(r)
	if s.wrong.blocked(from, time.Now()) {
		s.noteRefused(r, "", "the shared password", "too many wrong attempts from this source")
		tooMany(w)
		return
	}
	if !s.keeper.admits(r.FormValue("password")) {
		s.wrong.failed(from, time.Now())
		// What was typed is never written down, only that it was wrong.
		s.noteRefused(r, "", "the shared password", "the password was wrong")
		http.Redirect(w, r, "/login?wrong=1", http.StatusSeeOther)
		return
	}
	s.keeper.issue(w, r)
	s.noteSignIn(r, sharedSignIn, Admin, "the shared password")
	s.afterSignIn(w, r)
}

func (s *Server) signOut(w http.ResponseWriter, r *http.Request) {
	s.note(r, changelog.SignedOut, nil)
	who := s.keeper.who(r)
	s.keeper.end(r)
	s.keeper.clear(w, r)
	if s.front != nil {
		http.Redirect(w, r, s.front.exit(who), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// place writes a task's directory and takes it into the index. It is the same
// sequence whichever page asked for the task, and the caller wakes the queue
// once it has recorded whatever else the task belongs to. A turn is in the
// workspace of its instance.
func (s *Server) place(spec task.Spec, record tasks.Record) error {
	if held, found := s.instances.Get(record.Instance); found {
		record.Workspace = workspace.Stored(held.Workspace)
	}
	spec.Workspace = record.Workspace
	dir := task.Dir(filepath.Join(s.index.Root(), record.ID))
	if err := dir.Make(); err != nil {
		return err
	}
	if spec.Person == "" {
		spec.Person = record.Asked
	}
	// A room's agent works with its owner's tools, which the room's admin accepted with it.
	if err := s.equip(&spec, dir, record.Payer()); err != nil {
		return err
	}
	if err := s.giveConnectors(&spec, dir, record); err != nil {
		return err
	}
	spec.Brief += s.roomAppsLine(record, spec)
	if err := spec.Write(dir); err != nil {
		return err
	}
	if err := harness.Keep(dir, spec.Harness); err != nil {
		return err
	}
	return s.index.Add(record)
}

// turnAway sends somebody back to the page they came from, carrying the reason
// they did not get what they asked for.
func (s *Server) turnAway(w http.ResponseWriter, r *http.Request, back, why string) {
	http.Redirect(w, r, s.withMessage(back, "refused", why), http.StatusSeeOther)
}

// file hands back one record file. The name is checked against the listing
// rather than cleaned, so nothing outside out/ can be asked for. A person's browser is given the file in a page of the app, with the
// way back to the record, so that the window of the desktop app, which has no address bar, has one; a program or a download is given
// the bytes as they are.
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	_, dir, name, found := s.recordFileAsked(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("raw") == "" && strings.Contains(r.Header.Get("Accept"), "text/html") && shownAsPage(dir.OutFile(name)) {
		s.appPage(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, dir.OutFile(name))
}

// recordFileAsked is the record and the file of it the address names, when the person may see them.
func (s *Server) recordFileAsked(r *http.Request) (tasks.Record, task.Dir, string, bool) {
	record, dir, found := s.find(r)
	name := r.PathValue("name")
	if !found || !slicesContains(listing(dir), name) {
		return tasks.Record{}, "", "", false
	}
	return record, dir, name, true
}

// shownAsPage is whether a run's file is drawn as a page: one that is text is, and anything else is given as it is.
func shownAsPage(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	head, _ := readSome(path, 8<<10)
	switch kindOf(filepath.Base(path), head) {
	case kindText, kindMarkdown, kindTable, kindHTML:
		return true
	}
	return false
}

// recordFileData is one of a run's files as the viewer draws it: as text, whatever it is, under the record it is part of.
func (s *Server) recordFileData(r *http.Request) (map[string]any, error) {
	record, dir, name, found := s.recordFileAsked(r)
	if !found || !shownAsPage(dir.OutFile(name)) {
		return nil, failed{http.StatusNotFound, "there is no such file"}
	}
	path := dir.OutFile(name)
	info, err := os.Stat(path)
	if err != nil {
		return nil, failed{http.StatusNotFound, "there is no such file"}
	}
	here := "/jobs/" + record.ID + "/files/" + url.PathEscape(name)
	file := viewedFile{Name: name, Kind: kindText, Size: measure(info.Size()), Modified: info.ModTime().Unix(), Raw: here + "?raw=1", Download: here + "?raw=1&download=1"}
	s.fill(&file, path, info.Size())
	return map[string]any{
		"File": file, "Back": "/jobs/" + record.ID, "BackWord": "Back to the record", "Prose": "", "Save": "", "Source": "",
		"Crumbs": []pageCrumb{{Label: "Activity", Href: "/jobs"}, {Label: "Details", Href: "/jobs/" + record.ID}},
	}, nil
}

func (s *Server) find(r *http.Request) (tasks.Record, task.Dir, bool) {
	record, found := s.index.Get(r.PathValue("id"))
	if !found || !s.mayUseRecord(r, record) {
		return tasks.Record{}, "", false
	}
	return record, record.Dir(s.index.Root()), true
}

func slicesContains(all []string, want string) bool {
	for _, one := range all {
		if one == want {
			return true
		}
	}
	return false
}

// asJSON is here so a script can read the same list the pages show.
func (s *Server) asJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
