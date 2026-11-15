package web

import (
	"fmt"
	"maps"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/spend"
	"github.com/getlatentic/gusto/pkg/task"
)

// turn is one thing said and what came back of it.
type turn struct {
	tasks.Record
	Said    string
	Replied string
	// Steps is what the harness did on the way to its reply, and Changed the
	// files it left different.
	Steps   []step
	Changed []string
	Deleted []string
	// Files is how a conversation draws Changed: a card for each file, read from the agent's folder.
	Files *turnFiles
	// Previews are the folders among Changed that hold a page, which can be looked at.
	Previews []string
	// Spent is the proxy's count of what the turn's calls cost.
	Spent   spend.Tally
	Metered bool
	// Stopped is the harness's last words when the turn failed, which is the
	// reason as near as the record has one.
	Stopped string
	// Home is set when the person's kept home being full is why the turn did not run or was stopped.
	Home *fullHome
	// Capped is the spending limit that refused the turn's model, in the proxy's own words, when one did, and Limit the same
	// in a sentence for the people reading the turn.
	Capped *spend.Refusal
	Limit  string
	// Blocked is how many requests to other sites the proxy refused, and Refused
	// the sites they were for: the guarantee, shown where the work is rather than a page away.
	Blocked int
	Refused []blocked
	// Apps says whose connected apps a room turn could use, shown with its steps.
	Apps string
	roomTurn
}

// conversationState is a conversation as it is now, read for one person: its turns in their places, and where the room's stream
// and version stood when they were read.
type conversationState struct {
	held    instance.Instance
	thread  instance.Thread
	stream  *liveStream
	version string
	turns   []turn
	// running is the turn that has not finished, or the first of a room's that has not.
	running string
}

// conversationOf reads the conversation the path names, for the person asking. Whoever opens a room has read what it shows, up
// to its newest message.
func (s *Server) conversationOf(r *http.Request) (conversationState, bool) {
	held, found := s.shared(r)
	if !found {
		return conversationState{}, false
	}
	state := conversationState{held: s.liveRoom(held)}
	// The room's place in its event stream is read before its conversation, so everything the page shows is at least as new as
	// the position it follows from.
	state.stream = s.streamOf(state.held)
	state.thread, found = s.instances.Thread(held.ID, r.PathValue("thread"))
	if !found {
		return conversationState{}, false
	}
	// Its version is read first too: read after the turns, it could stand for a change they do not show.
	newest, version := s.activityOfRoom(state.held, state.thread)
	state.version = version
	state.turns = s.fullTurns(state.held, state.thread)
	if len(state.turns) > 0 && !state.turns[len(state.turns)-1].State.Done() {
		state.running = state.turns[len(state.turns)-1].ID
	}
	if state.held.Room {
		// The room is written only when its newest message moved.
		_ = s.instances.Saw(held.ID, state.thread.ID, s.askerName(r), newest)
		state.turns = s.roomTimeline(state.held, state.thread, state.turns)
		state.running = roomWaiting(state.turns)
	}
	return state, true
}

// fullTurns is every turn of a conversation as the page shows it, with the files each produced.
func (s *Server) fullTurns(held instance.Instance, thread instance.Thread) []turn {
	var turns []turn
	ledger, _ := s.spending()
	for _, id := range thread.Turns {
		record, found := s.index.Get(id)
		if !found {
			continue
		}
		shown := s.fullTurn(record, ledger)
		shown.Files = s.fileChips(held.ID, shown.Changed, shown.Deleted)
		turns = append(turns, shown)
	}
	return turns
}

// hasConversation says whether the path names a conversation the person asking may open, so the page of one that is not there
// is a 404 and not the app.
func (s *Server) hasConversation(r *http.Request) bool {
	held, found := s.shared(r)
	if !found {
		return false
	}
	_, found = s.instances.Thread(held.ID, r.PathValue("thread"))
	return found
}

// conversationData is the conversation page's data: what has been said in one thread, and somewhere to say more.
func (s *Server) conversationData(r *http.Request) (map[string]any, error) {
	state, found := s.conversationOf(r)
	if !found {
		return nil, failed{http.StatusNotFound, "there is no such conversation"}
	}
	held, thread := state.held, state.thread
	answers := s.agentOf(held)
	offered := s.offeredTo(r)
	_, owns := s.owned(r)
	liveApps, starting := previewing(s.appsOf(held.ID))
	page := map[string]any{
		"Instance":    instanceViewOf(held),
		"Thread":      threadView{ID: thread.ID, Dir: thread.Dir},
		"Named":       s.nameOf(thread),
		"Path":        threadPath(thread),
		"Me":          s.askerName(r),
		"Agent":       agentViewOf(answers),
		"DirectAgent": directAgentOf(held),
		"Turns":       s.turnViews(state.turns, s.sceneOf(held, thread)),
		"Starters":    s.startersOf(held),
		"Several":     len(offered) > 1,
		"Models":      s.modelOptionsOf(offered),
		"Sharing":     s.apps != nil && owns,
		"Owns":        owns,
		"Apps":        s.appLinks(liveApps),
		"Starting":    starting,
		"Waiting":     !held.Room && state.running != "",
		"Latest":      state.running,
		"Holding":     s.holdingOfLatest(held, state.running),
		"Hint":        hintOf(held, answers),
	}
	maps.Copy(page, s.roomParts(r, state))
	return page, nil
}

// roomParts is what the page carries of a room: who can be mentioned, its stream, its settings and members, and what was
// refused in its dialogs.
func (s *Server) roomParts(r *http.Request, state conversationState) map[string]any {
	held := state.held
	return map[string]any{
		"Mentions":          s.mentionables(held, s.askerName(r)),
		"Names":             s.roomNames(held),
		"Stream":            state.stream,
		"Version":           state.version,
		"Headcount":         len(held.Members) + len(held.Agents),
		"Settings":          s.roomSettingsOf(r, held),
		"Members":           s.roomMembersOf(r, held),
		"Guidelines":        held.Guidelines,
		"MaxGuidelines":     instance.MaxGuidelines,
		"Refused":           s.refusedIn(r, "members"),
		"AddRefused":        s.refusedIn(r, "add"),
		"GuidelinesRefused": s.refusedIn(r, "guidelines"),
		"Unsent":            s.refusedIn(r, ""),
		"AttachMost":        attachLimits.Files,
		"AttachBytes":       attachLimits.Bytes,
	}
}

// turnsData is the conversation's turns as they are now, for a page that was told something changed. Its headers say where the
// conversation and its room's settings stood, so the page can tell whether it has to look again.
func (s *Server) turnsData(w http.ResponseWriter, r *http.Request) {
	state, found := s.conversationOf(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	settings := roomSettingsMark(state.held)
	if state.version != "" {
		w.Header().Set("X-Room-Version", state.version)
	}
	w.Header().Set("X-Room-Settings", settings)
	w.Header().Set("Cache-Control", "no-store")
	s.asJSON(w, map[string]any{
		"Turns":    s.turnViews(state.turns, s.sceneOf(state.held, state.thread)),
		"Version":  state.version,
		"Settings": settings,
	})
}

// activityOfRoom is a room's newest change and version, and nothing for a conversation that is not a room's.
func (s *Server) activityOfRoom(held instance.Instance, thread instance.Thread) (int64, string) {
	if !held.Room {
		return 0, ""
	}
	return s.roomActivity(thread)
}

// fullTurn is a turn as the page shows it, read from the turn's own directory.
func (s *Server) fullTurn(entry tasks.Record, ledger *spend.Ledger) turn {
	dir := entry.Dir(s.index.Root())
	shown := turn{Record: entry, Said: entry.Brief, Replied: said(dir), Steps: stepsOf(dir)}
	shown.Changed, shown.Deleted = changes(dir)
	shown.Previews = previewsOf(shown.Changed)
	shown.Spent, shown.Metered = spentBy(ledger, entry.ID)
	if entry.State == tasks.Failed || entry.State == tasks.Interrupted {
		shown.Stopped = stopped(dir)
		shown.Home = homeFullOf(dir)
	}
	if refusal, err := spend.Stopped(dir.OutFile("proxy.jsonl"), entry.Started); err == nil {
		shown.Capped = refusal
	}
	switch {
	case shown.Capped != nil && shown.Spent.Calls == 0:
		// The model was never asked, so what the agent said is only the refusal again.
		shown.Replied = ""
	case shown.Capped != nil:
		// The page says what stopped the turn under the reply, so the agent printing the refusal again is left out.
		shown.Replied = withoutRefusal(shown.Replied)
	}
	shown.Limit = s.limitWords(entry, shown.Capped)
	if calls := callsOf(dir); calls != nil {
		shown.Blocked, shown.Refused = len(calls.Denied), calls.Sites()
	}
	shown.Apps = s.usedApps(entry, dir)
	return shown
}

// withoutRefusal is a reply without the lines where the agent repeats the proxy's refusal.
func withoutRefusal(replied string) string {
	var kept []string
	for _, line := range strings.Split(replied, "\n") {
		if !strings.Contains(line, proxy.Refusal) {
			kept = append(kept, line)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// said is what the harness replied, read from the turn's own transcript. It is
// bounded because a page is not the place to read a megabyte.
func said(dir task.Dir) string {
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

// say is one turn: an ordinary task that mounts the instance and names the
// thread, so the queue knows not to run it beside another turn in the same
// folder.
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
	if held.Room {
		s.roomSay(w, r, held, thread)
		return
	}
	brief := strings.TrimSpace(r.FormValue("said"))
	if brief == "" {
		http.Redirect(w, r, threadPath(thread), http.StatusSeeOther)
		return
	}
	record, _, err := s.startTurn(s.keeper.who(r), held, thread, brief, "")
	if err != nil {
		s.turnAway(w, r, threadPath(thread), err.Error())
		return
	}
	http.Redirect(w, r, threadPath(thread)+"#"+record.ID, http.StatusSeeOther)
}

// startTurn queues what somebody said in a thread as a task and records it there. With a request id it first looks for the
// task that same asker made with it, and gives that one back, so a send that was retried is one turn. It says whether the
// task was already there. Sends with an id go one at a time, so two of them arriving together cannot both find nothing.
func (s *Server) startTurn(asked string, held instance.Instance, thread instance.Thread, brief, key string) (tasks.Record, bool, error) {
	if key != "" {
		s.keyed.Lock()
		defer s.keyed.Unlock()
		if already, found := s.index.FindKey(asked, key); found {
			return already, true, nil
		}
	}
	spec := task.Spec{
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
		return tasks.Record{}, false, err
	}
	record := tasks.Record{
		ID: id, State: tasks.Queued,
		Brief: brief, Repo: held.Repo, Harness: held.Harness, Model: held.Model,
		Instance: held.ID, Thread: thread.ID,
		Asked: asked, Key: key, Team: spec.Team, Queued: tasks.Now(),
	}
	if err := s.place(spec, record); err != nil {
		return tasks.Record{}, false, err
	}
	if err := s.instances.Said(held.ID, thread.ID, record.ID, ""); err != nil {
		return tasks.Record{}, false, err
	}
	if err := s.instances.Name(held.ID, thread.ID, instance.TitleOf(brief)); err != nil {
		return tasks.Record{}, false, err
	}
	s.queue.Wake()
	return record, false, nil
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

// dialogs are the parts of a room's page that carry their own refusals: what was asked in one comes back with its name in the
// address.
var dialogs = []string{"members", "add", "guidelines"}

// refusedIn is why the last thing asked of the page was refused, when it was asked of the part named: a dialog, or with no name,
// what was said in the conversation, which carries no dialog's name.
func (s *Server) refusedIn(r *http.Request, part string) string {
	asked := r.URL.Query()
	if part == "" {
		for _, dialog := range dialogs {
			if asked.Has(dialog) {
				return ""
			}
		}
	} else if !asked.Has(part) {
		return ""
	}
	return s.refusal(r)
}

// limitWords is the limit that stopped a turn, said plainly: a person's budget by their name and the month, or the proxy's words.
func (s *Server) limitWords(record tasks.Record, refusal *spend.Refusal) string {
	if refusal == nil || (refusal.Cap == "credit" && refusal.SpentUSD >= refusal.LimitUSD) {
		return ""
	}
	if refusal.Cap == "person" && record.Asked != "" {
		month := time.Unix(record.Started, 0).UTC().Format("January")
		return fmt.Sprintf("Stopped before asking the model: %s's budget for %s is used up, $%.2f of $%.2f.", s.personName(record.Asked), month, refusal.SpentUSD, refusal.LimitUSD)
	}
	return "The model was not asked: " + refusal.Reason + "."
}
