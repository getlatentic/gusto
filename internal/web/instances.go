package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
)

// running is an instance as a list of them shows it: how much is being said to
// it, where to pick that up, and what it was last doing.
type running struct {
	instance.Instance
	Threads int
	Where   string
	Latest  jobs.Record
}

// catalogPage is the front door: the harnesses this deployment can actually
// start, and the ones already started.
func (s *Server) catalogPage(w http.ResponseWriter, r *http.Request) {
	held := s.instances.List()
	listed := make([]running, 0, len(held))
	for _, each := range held {
		listed = append(listed, s.lately(each))
	}
	s.render(w, "catalog.html", map[string]any{
		"Title":     "haas",
		"Nav":       "harnesses",
		"Catalog":   s.catalog,
		"Models":    s.models,
		"Instances": listed,
		"Refused":   r.URL.Query().Get("refused"),
	})
}

// setUp is the step between picking a harness and having one: what it thinks
// with, and what it starts out working on.
func (s *Server) setUp(w http.ResponseWriter, r *http.Request) {
	wanted := r.URL.Query().Get("harness")
	for _, entry := range s.catalog {
		if entry.Name == wanted && entry.Ready {
			s.render(w, "setup.html", map[string]any{
				"Title":   "haas — " + entry.Name,
				"Nav":     "harnesses",
				"Harness": entry,
				"Models":  s.models,
				"Then":    r.URL.Query().Get("then"),
				"Refused": r.URL.Query().Get("refused"),
			})
			return
		}
	}
	s.turnAway(w, r, "/", "this deployment cannot start "+wanted)
}

// lately is where to pick an instance up and what it was last doing. Somebody
// coming back wants the conversation, not the page about it.
func (s *Server) lately(held instance.Instance) running {
	threads := s.instances.Threads(held.ID)
	shown := running{Instance: held, Threads: len(threads), Where: "/instances/" + held.ID}
	if len(threads) == 0 {
		return shown
	}
	last := threads[len(threads)-1]
	shown.Where = threadPath(last)
	shown.Latest = s.latestTurn(last)
	return shown
}

// startInstance gives a harness a computer to work in.
func (s *Server) startInstance(w http.ResponseWriter, r *http.Request) {
	wanted := instance.Instance{
		Harness: r.FormValue("harness"),
		Model:   strings.TrimSpace(r.FormValue("model")),
		Repo:    strings.TrimSpace(r.FormValue("repo")),
		Who:     s.keeper.who(r),
	}
	if wanted.Harness == "" {
		s.turnAway(w, r, "/", "pick a harness to start")
		return
	}
	// Starting from the catalog names no model, and the deployment's first is
	// what the page would have offered anyway.
	if wanted.Model == "" && len(s.models) > 0 {
		wanted.Model = s.models[0]
	}
	name := wanted.Harness
	if wanted.Repo != "" {
		name = filepath.Base(strings.TrimSuffix(wanted.Repo, ".git"))
	}
	started, err := s.instances.Start(s.index.Free(name, time.Now()), wanted)
	if err != nil {
		s.turnAway(w, r, "/", err.Error())
		return
	}
	// A one-off against a repository is an instance started for a task, and
	// whoever asked for it wants the task form, not a conversation.
	if r.FormValue("then") == "task" {
		http.Redirect(w, r, "/instances/"+started.ID+"#task", http.StatusSeeOther)
		return
	}
	// Somebody who started a harness wants to be talking to it, not reading a
	// page about it, so the first conversation is opened with it.
	thread, err := s.instances.NewThread(started.ID, "")
	if err != nil {
		http.Redirect(w, r, "/instances/"+started.ID, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, threadPath(thread), http.StatusSeeOther)
}

// instancePage is one computer: what it runs, what it works in, and every
// conversation being held with it.
func (s *Server) instancePage(w http.ResponseWriter, r *http.Request) {
	held, found := s.instances.Get(r.PathValue("id"))
	if !found {
		http.NotFound(w, r)
		return
	}
	threads := s.instances.Threads(held.ID)
	said := make([]map[string]any, 0, len(threads))
	for _, thread := range threads {
		said = append(said, map[string]any{"Thread": thread, "Latest": s.latestTurn(thread)})
	}
	s.render(w, "instance.html", map[string]any{
		"Title":     "haas — " + held.ID,
		"Nav":       "harnesses",
		"Instance":  held,
		"Threads":   said,
		"Workspace": s.instances.Workspace(held.ID),
		"Tasks":     s.tasks(held.ID),
		"Refused":   r.URL.Query().Get("refused"),
	})
}

// latestTurn is the last thing said in a thread, which is what a list of
// threads needs to tell them apart.
func (s *Server) latestTurn(thread instance.Thread) (record jobs.Record) {
	if len(thread.Turns) == 0 {
		return record
	}
	record, _ = s.index.Get(thread.Turns[len(thread.Turns)-1])
	return record
}

// openThread starts another conversation on an instance that is already there.
func (s *Server) openThread(w http.ResponseWriter, r *http.Request) {
	held, found := s.instances.Get(r.PathValue("id"))
	if !found {
		http.NotFound(w, r)
		return
	}
	thread, err := s.instances.NewThread(held.ID, strings.TrimSpace(r.FormValue("title")))
	if err != nil {
		s.turnAway(w, r, "/instances/"+held.ID, err.Error())
		return
	}
	http.Redirect(w, r, threadPath(thread), http.StatusSeeOther)
}

func threadPath(thread instance.Thread) string {
	return "/instances/" + thread.Instance + "/threads/" + thread.ID
}

// conversation shows what has been said in one thread, and offers somewhere to
// say more.
func (s *Server) conversation(w http.ResponseWriter, r *http.Request) {
	page, found := s.conversationPage(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	s.render(w, "conversation.html", page)
}

// turn is one thing said and what came back of it.
type turn struct {
	jobs.Record
	Said    string
	Replied string
}

// conversationPage is what both the whole page and the fragment need, so a
// swapped-in turn cannot disagree with the page it lands in.
func (s *Server) conversationPage(r *http.Request) (map[string]any, bool) {
	held, found := s.instances.Get(r.PathValue("id"))
	if !found {
		return nil, false
	}
	thread, found := s.instances.Thread(held.ID, r.PathValue("thread"))
	if !found {
		return nil, false
	}
	var turns []turn
	for _, id := range thread.Turns {
		record, found := s.index.Get(id)
		if !found {
			continue
		}
		turns = append(turns, turn{
			Record:  record,
			Said:    record.Brief,
			Replied: said(record.Dir(s.index.Root())),
		})
	}
	running := ""
	if len(turns) > 0 && !turns[len(turns)-1].State.Done() {
		running = turns[len(turns)-1].ID
	}
	named := thread.Title
	if named == "" {
		named = held.Harness
	}
	return map[string]any{
		// Every conversation on this instance, because one instance holds as
		// many as somebody wants and the page you are on is the one place you
		// would go looking for the others.
		"Threads":  s.instances.Threads(held.ID),
		"Title":    "haas — " + named,
		"Nav":      "harnesses",
		"Instance": held,
		"Thread":   thread,
		"Path":     threadPath(thread),
		"Turns":    turns,
		"Waiting":  running != "",
		"Latest":   running,
	}, true
}

// said is what the harness replied, read from the turn's own transcript. It is
// bounded because a page is not the place to read a megabyte.
func said(dir job.Dir) string {
	text, err := os.ReadFile(dir.OutFile("transcript.log"))
	if err != nil {
		return ""
	}
	const most = 40 * 1024
	if len(text) > most {
		return "…" + string(text[len(text)-most:])
	}
	return string(text)
}

// turns is the conversation so far, on its own, so a finished turn can be
// swapped in without throwing away everything already on the page.
func (s *Server) turns(w http.ResponseWriter, r *http.Request) {
	page, found := s.conversationPage(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	s.render(w, "turns", page)
}

// say is one turn: an ordinary job that mounts the instance and names the
// thread, so the queue knows not to run it beside another turn of the same
// workspace.
func (s *Server) say(w http.ResponseWriter, r *http.Request) {
	held, found := s.instances.Get(r.PathValue("id"))
	if !found {
		http.NotFound(w, r)
		return
	}
	thread, found := s.instances.Thread(held.ID, r.PathValue("thread"))
	if !found {
		http.NotFound(w, r)
		return
	}
	brief := strings.TrimSpace(r.FormValue("said"))
	if brief == "" {
		http.Redirect(w, r, threadPath(thread), http.StatusSeeOther)
		return
	}
	spec := job.Spec{
		Brief:      brief,
		Harness:    held.Harness,
		Model:      held.Model,
		Session:    held.ID,
		Thread:     thread.ID,
		Remembered: thread.Remembered,
		Repo:       held.Repo,
	}
	record := jobs.Record{
		ID: s.index.Free(brief, time.Now()), State: jobs.Queued,
		Brief: brief, Repo: held.Repo, Harness: held.Harness, Model: held.Model,
		Instance: held.ID, Thread: thread.ID,
		Asked: s.keeper.who(r), Queued: jobs.Now(),
	}
	if err := s.place(spec, record); err != nil {
		s.turnAway(w, r, threadPath(thread), err.Error())
		return
	}
	if err := s.instances.Said(held.ID, thread.ID, record.ID, ""); err != nil {
		s.turnAway(w, r, threadPath(thread), err.Error())
		return
	}
	s.queue.Wake()
	http.Redirect(w, r, threadPath(thread)+"#"+record.ID, http.StatusSeeOther)
}
