package web

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/pkg/spend"
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
	// Steps is what the harness did on the way to its reply, and Changed the
	// files it left different.
	Steps   []step
	Changed []string
	// Previews are the folders among Changed that hold a page, which can be looked at.
	Previews []string
	// Spent is the proxy's count of what the turn's calls cost.
	Spent   spend.Tally
	Metered bool
	// Stopped is the harness's last words when the turn failed, which is the
	// reason as near as the record has one.
	Stopped string
	// Blocked is how many requests to other sites the proxy refused, and
	// Signed whether the turn's record is signed: the guarantees, shown where
	// the work is rather than a page away.
	Blocked int
	Signed  bool
	roomTurn
}

// conversationPage is what both the whole page and the fragment need, so a
// swapped-in turn cannot disagree with the page it lands in.
func (s *Server) conversationPage(r *http.Request) (map[string]any, bool) {
	held, found := s.shared(r)
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
		shown := turn{Record: record, Said: record.Brief, Replied: said(dir), Steps: stepsOf(dir), Changed: changed(dir)}
		shown.Previews = previewsOf(shown.Changed)
		shown.Spent, shown.Metered = spentBy(ledger, record.ID)
		if record.State == jobs.Failed || record.State == jobs.Interrupted {
			shown.Stopped = stopped(dir)
		}
		if calls := callsOf(dir); calls != nil {
			shown.Blocked = len(calls.Denied)
		}
		attested, err := evidence.Attest(dir, s.keyring)
		shown.Signed = err == nil && attested.Signed
		turns = append(turns, shown)
	}
	liveApps, appsStarting := previewing(s.appsOf(held.ID))
	running := ""
	if len(turns) > 0 && !turns[len(turns)-1].State.Done() {
		running = turns[len(turns)-1].ID
	}
	if held.Room {
		// Whoever has the room open has read what it shows, so their unread count starts again from here.
		_ = s.instances.Saw(held.ID, thread.ID, s.askerName(r), time.Now().Unix())
		turns = s.roomTimeline(held, thread, turns)
		running = roomWaiting(turns)
	}
	_, owns := s.owned(r)
	return map[string]any{
		// Every conversation on this instance, because one instance holds as
		// many as somebody wants and the page you are on is the one place you
		// would go looking for the others.
		"Threads":  s.namedThreads(held.ID),
		"Named":    s.nameOf(thread),
		"Agent":    s.agentOf(held),
		"Starters": s.startersOf(held),
		"Models":   asJSON(s.modelOptionsOf(s.offeredTo(r))),
		"Several":  len(s.offeredTo(r)) > 1,
		"Rail":     s.railFor(r, threadPath(thread)),
		"Here":     threadPath(thread),
		// A chat fills the window rather than sitting in a document.
		"Chat":     true,
		"Title":    productName + " — " + s.nameOf(thread),
		"Nav":      "chat",
		"Instance": held,
		"Thread":   thread,
		"Path":     threadPath(thread),
		"Turns":    turns,
		"Sharing":  s.apps != nil && owns,
		"Owns":     owns,
		"Me":       s.askerName(r),
		"Mentions": asJSON(s.mentionables(held)),
		"Hint":     composerHint(held),
		"Apps":     liveApps,
		"Starting": appsStarting,
		// While a preview starts the page looks again, so its link appears without being asked for.
		"Refresh": appsStarting,
		"Waiting": running != "",
		"Latest":  running,
	}, true
}

// said is what the harness replied, read from the turn's own transcript. It is
// bounded because a page is not the place to read a megabyte.
func said(dir job.Dir) string {
	if !dir.PlainOut("transcript.log") {
		return ""
	}
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
	held, found := s.sharedOr404(w, r)
	if !found {
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
	if held.Room {
		s.roomSay(w, r, held, thread, brief)
		return
	}
	record, _, err := s.startTurn(s.keeper.who(r), held, thread, brief, "")
	if err != nil {
		s.turnAway(w, r, threadPath(thread), err.Error())
		return
	}
	http.Redirect(w, r, threadPath(thread)+"#"+record.ID, http.StatusSeeOther)
}

// startTurn queues what somebody said in a thread as a job and records it there. With a request id it first looks for the
// job that same asker made with it, and gives that one back, so a send that was retried is one turn. It says whether the
// job was already there. Sends with an id go one at a time, so two of them arriving together cannot both find nothing.
func (s *Server) startTurn(asked string, held instance.Instance, thread instance.Thread, brief, key string) (jobs.Record, bool, error) {
	if key != "" {
		s.keyed.Lock()
		defer s.keyed.Unlock()
		if already, found := s.index.FindKey(asked, key); found {
			return already, true, nil
		}
	}
	spec := job.Spec{
		Brief:       brief,
		Harness:     held.Harness,
		Model:       held.Model,
		Session:     held.ID,
		Thread:      thread.ID,
		Dir:         thread.Dir,
		Remembered:  thread.Remembered,
		Repo:        held.Repo,
		Setup:       setupOf(held),
		Sensitivity: sensitivityOf(held),
	}
	id, err := s.index.Reserve(brief, time.Now())
	if err != nil {
		return jobs.Record{}, false, err
	}
	record := jobs.Record{
		ID: id, State: jobs.Queued,
		Brief: brief, Repo: held.Repo, Harness: held.Harness, Model: held.Model,
		Instance: held.ID, Thread: thread.ID,
		Asked: asked, Key: key, Team: spec.Team, Queued: jobs.Now(),
	}
	if err := s.place(spec, record); err != nil {
		return jobs.Record{}, false, err
	}
	if err := s.instances.Said(held.ID, thread.ID, record.ID, ""); err != nil {
		return jobs.Record{}, false, err
	}
	if err := s.instances.Name(held.ID, thread.ID, instance.TitleOf(brief)); err != nil {
		return jobs.Record{}, false, err
	}
	s.queue.Wake()
	return record, false, nil
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
