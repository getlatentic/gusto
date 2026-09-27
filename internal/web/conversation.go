package web

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/runner"
	"github.com/getlatentic/haas/internal/spend"
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
	// Steps is what the harness did on the way to its reply.
	Steps []step
	// Spent is the proxy's count of what the turn's calls cost.
	Spent   spend.Tally
	Metered bool
	// Stopped is the harness's last words when the turn failed, which is the
	// reason as near as the record has one.
	Stopped string
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
	ledger, _ := s.spending()
	for _, id := range thread.Turns {
		record, found := s.index.Get(id)
		if !found {
			continue
		}
		dir := record.Dir(s.index.Root())
		shown := turn{Record: record, Said: record.Brief, Replied: said(dir), Steps: stepsOf(dir)}
		shown.Spent, shown.Metered = spentBy(ledger, record.ID)
		if record.State == jobs.Failed || record.State == jobs.Interrupted {
			shown.Stopped = stopped(dir)
		}
		turns = append(turns, shown)
	}
	running := ""
	if len(turns) > 0 && !turns[len(turns)-1].State.Done() {
		running = turns[len(turns)-1].ID
	}
	return map[string]any{
		// Every conversation on this instance, because one instance holds as
		// many as somebody wants and the page you are on is the one place you
		// would go looking for the others.
		"Threads": s.namedThreads(held.ID),
		"Named":   s.nameOf(thread),
		// A chat fills the window rather than sitting in a document.
		"Chat":     true,
		"Title":    "haas — " + s.nameOf(thread),
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
	if err := s.instances.Name(held.ID, thread.ID, instance.TitleOf(brief)); err != nil {
		s.turnAway(w, r, threadPath(thread), err.Error())
		return
	}
	s.queue.Wake()
	http.Redirect(w, r, threadPath(thread)+"#"+record.ID, http.StatusSeeOther)
}

// step is one thing a harness did during a turn, as a person reads it.
type step struct {
	What   string
	Detail string
}

// stepsOf names what a turn did: a tool a server offered by its server and
// its name, a skill by the skill, and anything else by the tool and what it
// was given.
func stepsOf(dir job.Dir) []step {
	var steps []step
	for _, done := range ranBy(dir) {
		shown := step{What: strings.ToLower(done.Tool), Detail: done.Says()}
		if server, tool, found := strings.Cut(strings.TrimPrefix(done.Tool, "mcp__"), "__"); found && strings.HasPrefix(done.Tool, "mcp__") {
			shown = step{What: server, Detail: strings.ReplaceAll(tool, "_", " ")}
		}
		if len(shown.Detail) > 160 {
			shown.Detail = shown.Detail[:160] + "…"
		}
		steps = append(steps, shown)
	}
	return steps
}

// stopped is the last thing the harness or the runner said on its way out.
func stopped(dir job.Dir) string {
	last := strings.TrimSpace(runner.Tail(dir.OutFile("stderr.log"), 1))
	if last == "" {
		last = strings.TrimSpace(runner.Tail(dir.OutFile("transcript.log"), 1))
	}
	return last
}

// named is a conversation as a list shows it: its title, or the start of what
// was first said in it, or new when nothing has been.
type named struct {
	instance.Thread
	Name string
}

func (s *Server) namedThreads(id string) []named {
	threads := s.instances.Threads(id)
	shown := make([]named, 0, len(threads))
	for _, thread := range threads {
		shown = append(shown, named{Thread: thread, Name: s.nameOf(thread)})
	}
	return shown
}

func (s *Server) nameOf(thread instance.Thread) string {
	if thread.Title != "" {
		return thread.Title
	}
	if len(thread.Turns) > 0 {
		if first, found := s.index.Get(thread.Turns[0]); found {
			return instance.TitleOf(first.Brief)
		}
	}
	return "New thread"
}
