package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

var helpers = template.FuncMap{
	"modelName":  modelName,
	"mentions":   mentions,
	"personName": personName,
	"initials":   initials,
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
	// split is a list text, such as "send jobs, read spending", as its items.
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
	"size":   measure,
	"span":   func(seconds int64) string { return (time.Duration(seconds) * time.Second).String() },
	"hourOf": func(unix int64) string { return time.Unix(unix, 0).Format("15:04") },
	// outcome is how a run ended, in the words of a log: a turn replied, a
	// task passed or failed its gate, and one still going says so.
	"outcome": func(shown run) string {
		switch {
		case shown.State == jobs.Queued:
			return "waiting"
		case shown.State == jobs.Running:
			return "working"
		case shown.State == jobs.Interrupted:
			return "interrupted"
		case shown.Thread != "" && shown.State == jobs.Passed:
			return "replied"
		case shown.Thread != "":
			return "failed"
		case shown.State == jobs.Passed:
			return "check passed"
		}
		return "check failed"
	},
	"has": func(list []string, item string) bool { return slices.Contains(list, item) },
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
	"took": func(from, to int64) string {
		if from == 0 || to == 0 {
			return ""
		}
		return (time.Duration(to-from) * time.Second).String()
	},
}

// headlineUSD is an amount of money as a headline shows it: cents once there are
// dollars, and the fractions of a cent an agent's calls cost until then.
func headlineUSD(amount float64) string {
	if amount >= 1 {
		return fmt.Sprintf("$%.2f", amount)
	}
	return fmt.Sprintf("$%.4f", amount)
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if s.keeper.knows(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	wrong := r.URL.Query().Get("wrong") != ""
	s.render(w, "login.html", map[string]any{
		"Wrong":    wrong,
		"Local":    s.local != nil,
		"Refused":  r.URL.Query().Get("refused"),
		"Identity": s.identity.Configured(),
		"Provider": s.identity.Configured(),
		"Desktop":  r.URL.Query().Has("desktop"),
		// With a provider the shared password is the way in when the provider
		// is not, so it stays reachable and stops being the second thing on
		// the page. A wrong one keeps it open, or there is nowhere to try again.
		"Shared": !s.identity.Configured() || wrong || r.URL.Query().Has("shared"),
	})
}

func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	from := clientOf(r)
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
	s.keeper.end(r)
	s.keeper.clear(w, r)
	if s.front != nil {
		http.Redirect(w, r, s.front.URL+"/org", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// place writes a job's directory and takes it into the index. It is the same
// sequence whichever page asked for the job, and the caller wakes the queue
// once it has recorded whatever else the job belongs to.
func (s *Server) place(spec job.Spec, record jobs.Record) error {
	dir := job.Dir(filepath.Join(s.index.Root(), record.ID))
	if err := dir.Make(); err != nil {
		return err
	}
	if err := s.equip(&spec, dir, record.Asked); err != nil {
		return err
	}
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
	join := "?"
	if strings.Contains(back, "?") {
		join = "&"
	}
	http.Redirect(w, r, back+join+"refused="+template.URLQueryEscaper(why), http.StatusSeeOther)
}

// file hands back one evidence file. The name is checked against the listing
// rather than cleaned, so nothing outside out/ can be asked for.
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	record, dir, found := s.find(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	if !slicesContains(listing(dir), name) {
		http.NotFound(w, r)
		return
	}
	// A person's browser is given the file in a page, with the way back to the record, and a program or a download the bytes as they are.
	if r.URL.Query().Get("raw") == "" && strings.Contains(r.Header.Get("Accept"), "text/html") && s.evidencePage(w, r, record, dir, name) {
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, dir.OutFile(name))
}

// evidencePage draws one of a run's files as a page of the app, so that the window of the desktop app, which has no address bar, has a way
// back to the record. It says whether it did: a file that is not text is not drawn, and is given as it is.
func (s *Server) evidencePage(w http.ResponseWriter, r *http.Request, record jobs.Record, dir job.Dir, name string) bool {
	path := dir.OutFile(name)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	head, _ := readSome(path, 8<<10)
	switch kindOf(name, head) {
	case kindText, kindMarkdown, kindTable, kindHTML:
	default:
		return false
	}
	here := "/jobs/" + record.ID + "/files/" + url.PathEscape(name)
	file := viewedFile{Name: name, Kind: kindText, Size: measure(info.Size()), Modified: info.ModTime().Unix(), Raw: here + "?raw=1", Download: here + "?raw=1&download=1"}
	s.fill(&file, path, info.Size())
	if r.URL.Query().Get("panel") != "" {
		s.render(w, "file-panel", map[string]any{"File": file, "Rail": nil, "Page": here})
		return true
	}
	s.render(w, "file.html", map[string]any{
		"Title": productName + " — " + name, "Nav": "jobs", "File": file, "Evidence": record.ID,
		"Back": "/jobs/" + record.ID, "BackWord": "Back to the record",
	})
	return true
}

func (s *Server) find(r *http.Request) (jobs.Record, job.Dir, bool) {
	record, found := s.index.Get(r.PathValue("id"))
	if !found || !s.mayUseRecord(r, record) {
		return jobs.Record{}, "", false
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
