package web

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
)

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
		"Threads": s.instances.Threads(held.ID),
		// A chat fills the window rather than sitting in a document.
		"Chat":     true,
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
		Setup:      setupOf(held),
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
