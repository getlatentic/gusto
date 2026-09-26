package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
)

var helpers = template.FuncMap{
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
	"took": func(from, to int64) string {
		if from == 0 || to == 0 {
			return ""
		}
		return (time.Duration(to-from) * time.Second).String()
	},
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if s.keeper.knows(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.render(w, "login.html", map[string]any{
		"Wrong":    r.URL.Query().Get("wrong") != "",
		"Refused":  r.URL.Query().Get("refused"),
		"Identity": s.identity.Configured(),
	})
}

func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	if !s.keeper.admits(r.FormValue("password")) {
		http.Redirect(w, r, "/login?wrong=1", http.StatusSeeOther)
		return
	}
	s.keeper.issue(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) signOut(w http.ResponseWriter, r *http.Request) {
	s.keeper.clear(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	s.render(w, "list.html", map[string]any{
		"Title":     "haas",
		"Subtitle":  "a job is a directory",
		"Jobs":      s.index.List(),
		"Harnesses": catalog.Ready(s.catalog),
		"Catalog":   s.catalog,
		"Models":    s.models,
		"Refused":   r.URL.Query().Get("refused"),
	})
}

// submit writes the same job.json the command line writes, and nothing else. A
// job made here runs on any runner that can read the directory.
func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	gate := job.Words(r.FormValue("gate"))
	spec := job.Spec{
		Brief:       strings.TrimSpace(r.FormValue("brief")),
		Repo:        strings.TrimSpace(r.FormValue("repo")),
		Gate:        gate,
		Harness:     r.FormValue("harness"),
		Model:       strings.TrimSpace(r.FormValue("model")),
		Sensitivity: job.Sensitivity(r.FormValue("sensitivity")),
		Team:        strings.TrimSpace(r.FormValue("team")),
	}
	if spec.Brief == "" || spec.Repo == "" || len(gate) == 0 {
		http.Redirect(w, r, "/?refused=a+job+needs+a+repository,+a+brief+and+a+gate", http.StatusSeeOther)
		return
	}
	capped, err := dollars(r.FormValue("usd"))
	if err == nil {
		spec.Budget.USD = capped
		err = spec.Budget.Check()
	}
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	id := s.index.Free(spec.Brief, time.Now())
	dir := job.Dir(filepath.Join(s.index.Root(), id))
	if err := dir.Make(); err != nil {
		s.refuse(w, r, err)
		return
	}
	if err := spec.Write(dir); err != nil {
		s.refuse(w, r, err)
		return
	}
	if err := harness.Keep(dir, spec.Harness); err != nil {
		s.refuse(w, r, err)
		return
	}
	record := jobs.Record{
		ID: id, State: jobs.Queued, Brief: spec.Brief, Repo: spec.Repo,
		Harness: spec.Harness, Model: spec.Model, Asked: s.keeper.who(r),
		Queued: jobs.Now(),
	}
	if err := s.index.Add(record); err != nil {
		s.refuse(w, r, err)
		return
	}
	s.queue.Wake()
	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}

// dollars is a cap a person typed. Nothing typed is no cap, which is not the
// same as a cap of nothing.
func dollars(typed string) (*float64, error) {
	typed = strings.TrimPrefix(strings.TrimSpace(typed), "$")
	if typed == "" {
		return nil, nil
	}
	amount, err := strconv.ParseFloat(typed, 64)
	if err != nil {
		return nil, fmt.Errorf("%q is not an amount of dollars", typed)
	}
	return &amount, nil
}

// turnAway sends somebody back to the page they came from, carrying the reason
// they did not get what they asked for.
func (s *Server) turnAway(w http.ResponseWriter, r *http.Request, back, why string) {
	http.Redirect(w, r, back+"?refused="+template.URLQueryEscaper(why), http.StatusSeeOther)
}

func (s *Server) refuse(w http.ResponseWriter, r *http.Request, err error) {
	s.turnAway(w, r, "/", err.Error())
}

func (s *Server) job(w http.ResponseWriter, r *http.Request) {
	record, dir, found := s.find(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	page := map[string]any{"Job": record, "Title": "haas — " + record.ID, "Subtitle": record.ID}
	if spec, err := job.Read(dir); err == nil {
		page["Spec"] = spec
	}
	manifest := readManifest(dir)
	drawEvidence(page, record, dir, manifest)
	page["Files"] = hashed(listing(dir), manifest)
	s.render(w, "job.html", page)
}

// hashedFile is one file in out/ beside the hash the manifest claims for it, so
// a reader can hash their own download and compare without fetching the manifest
// as well.
type hashedFile struct {
	Name   string
	SHA256 string
}

// hashed pairs what is on disk with what the manifest says. No hash means the
// manifest does not name the file: manifest.json, which cannot hash itself, and
// anything that appeared after the run, which `haas verify` reports.
func hashed(names []string, manifest *evidence.Manifest) []hashedFile {
	claimed := map[string]string{}
	for _, entry := range entriesOf(manifest) {
		claimed[entry.Name] = entry.SHA256
	}
	files := make([]hashedFile, 0, len(names))
	for _, name := range names {
		files = append(files, hashedFile{Name: name, SHA256: claimed[name]})
	}
	return files
}

func listing(dir job.Dir) []string {
	entries, err := os.ReadDir(dir.Out())
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

// file hands back one evidence file. The name is checked against the listing
// rather than cleaned, so nothing outside out/ can be asked for.
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	_, dir, found := s.find(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	name := r.PathValue("name")
	if !slicesContains(listing(dir), name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, dir.OutFile(name))
}

func (s *Server) find(r *http.Request) (jobs.Record, job.Dir, bool) {
	record, found := s.index.Get(r.PathValue("id"))
	if !found {
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
