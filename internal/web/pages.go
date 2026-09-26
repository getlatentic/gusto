package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
)

var helpers = template.FuncMap{
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
	s.render(w, "login.html", map[string]any{"Wrong": r.URL.Query().Get("wrong") != ""})
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
		"Harnesses": s.harnesses,
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
	}
	if spec.Brief == "" || spec.Repo == "" || len(gate) == 0 {
		http.Redirect(w, r, "/?refused=a+job+needs+a+repository,+a+brief+and+a+gate", http.StatusSeeOther)
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
		Harness: spec.Harness, Model: spec.Model, Asked: "the shared sign-in",
		Queued: jobs.Now(),
	}
	if err := s.index.Add(record); err != nil {
		s.refuse(w, r, err)
		return
	}
	s.queue.Wake()
	http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}

func (s *Server) refuse(w http.ResponseWriter, r *http.Request, err error) {
	http.Redirect(w, r, "/?refused="+template.URLQueryEscaper(err.Error()), http.StatusSeeOther)
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
	manifest, err := evidence.Read(dir)
	if err == nil {
		page["Manifest"] = manifest
	}
	if patch, err := os.ReadFile(dir.OutFile("diff.patch")); err == nil && len(patch) > 0 {
		page["Patch"] = string(patch)
	}
	if refusal := gateOutput(record, dir); refusal != "" {
		page["GateLog"] = refusal
	}
	page["Files"] = hashed(listing(dir), manifest)
	s.render(w, "job.html", page)
}

// gateOutput is what the gate printed, for a job the gate did not pass. That
// output is the answer a reader of a failure came for, and a pass has nothing to
// explain. A job still running is left to the stream.
func gateOutput(record jobs.Record, dir job.Dir) string {
	if !record.State.Done() || record.State == jobs.Passed {
		return ""
	}
	output, err := os.ReadFile(dir.OutFile("gate.log"))
	if err != nil {
		return ""
	}
	return string(output)
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
func hashed(names []string, manifest evidence.Manifest) []hashedFile {
	claimed := make(map[string]string, len(manifest.Files))
	for _, entry := range manifest.Files {
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
