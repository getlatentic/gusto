package web

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

// A task's page and its data, in the shapes web/src/pages/work/types.ts names: one run and everything it left, what it said,
// what it ran, what it changed, whether its check passed, what its calls to a model cost, and the record that vouches for it.

const taskRoute = "/jobs/{id}"

// workRoutes serves a task's page, which sends a person who is not an admin to the conversation a turn was said in, and its data.
func (s *Server) workRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+taskRoute, s.guard(s.taskPage))
	mux.HandleFunc("GET /app/api"+taskRoute, s.guard(s.appData(s.taskData)))
}

func (s *Server) taskPage(w http.ResponseWriter, r *http.Request) {
	record, _, found := s.find(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	if to := s.turnPlace(r, record); to != "" {
		http.Redirect(w, r, to, http.StatusSeeOther)
		return
	}
	s.appPage(w, r)
}

// turnPlace is where a turn is read by somebody who is not an admin: in its conversation, at the turn. It is empty for a task
// that is read on its own page.
func (s *Server) turnPlace(r *http.Request, record tasks.Record) string {
	if record.Thread == "" || s.admin(r) {
		return ""
	}
	if thread, found := s.instances.Thread(record.Instance, record.Thread); found {
		return threadPath(thread) + "#" + record.ID
	}
	return ""
}

// jobView is the task itself.
type jobView struct {
	ID, Brief, State, Harness, Repo, Model string
	Started, Ended                         int64
}

type taskAgentView struct{ ID, Name, Brain string }

// talkView is the conversation a turn belongs to, as its page places it: which message it was and the ones either side.
type talkView struct {
	Name          string
	Place, Of     int
	Before, After string
}

// cutView says that the page draws only the start of a record file, and where the whole of it is.
type cutView struct {
	Cut          bool
	Name, Raw    string
	Whole, Shown string
}

type transcriptView struct {
	Empty bool
	HTML  string
	cutView
}

type patchFilesView struct {
	Empty bool
	Files []diffFile
	cutView
}

type callsView struct {
	InputTokens, OutputTokens string
	Denied                    int
	Sites                     []blocked
}

type verdictView struct {
	Passed bool
	Code   int
}

type checkLogView struct {
	Text string
	cutView
}

type checkPageView struct {
	Verdict  *verdictView
	Command  string
	Log      *checkLogView
	Explains bool
}

type capStopView struct{ Cap, Reason string }

type proofView struct {
	Holds, Signed        bool
	Files                int
	Wrong                []string
	Signer, Unchecked    string
	Key, Bundle, Command string
}

// taskData is a task's page. A turn of a conversation, for somebody who is not an admin, is read in its conversation, and its
// data says only where.
func (s *Server) taskData(r *http.Request) (map[string]any, error) {
	found, dir, ok := s.find(r)
	if !ok {
		return nil, failed{http.StatusNotFound, "there is no such task"}
	}
	if to := s.turnPlace(r, found); to != "" {
		return map[string]any{"MovedTo": to}, nil
	}
	ledger, _ := s.spending()
	shown := s.recorded(found, ledger)
	spec, specErr := task.Read(dir)
	manifest := readManifest(dir)
	page := map[string]any{
		"Job":     jobView{ID: found.ID, Brief: found.Brief, State: string(found.State), Harness: found.Harness, Repo: found.Repo, Model: found.Model, Started: found.Started, Ended: found.Ended},
		"Outcome": outcomeOf(found),
		"Took":    took(found.Started, found.Ended),
		"Waiting": s.waiting(found.ID),
		"On":      shown.On,
		"Brain":   modelName(found.Model),
		"Admin":   s.admin(r),
		"Steps":   stepsOf(dir),
		"Calls":   callsViewOf(callsOf(dir)),
		"Files":   hashed(listing(dir), manifest),
		"Proof":   proofViewOf(s.proofOf(found.ID, dir, manifest)),
	}
	if shown.Metered {
		page["Cost"] = costOfRun(shown)
	}
	s.placeTask(page, found)
	s.recordParts(page, found, dir, manifest, specErr == nil && len(spec.Check) == 0)
	return page, nil
}

// placeTask adds the agent a task ran on and, for a turn, the conversation around it.
func (s *Server) placeTask(page map[string]any, found tasks.Record) {
	if held, ok := s.instances.Get(found.Instance); ok {
		named := s.agentOf(held)
		page["Agent"] = taskAgentView{ID: held.ID, Name: named.Name, Brain: named.Brain}
	}
	if found.Thread == "" {
		return
	}
	if thread, ok := s.instances.Thread(found.Instance, found.Thread); ok {
		page["ThreadPath"] = threadPath(thread)
		page["Talk"] = s.talkAround(thread, found.ID)
	}
}

// recordParts adds what the run left behind. The transcript and the check wait for the task to finish, because until then the
// live stream is the transcript and the check has not answered. A turn in a conversation names no check, so it has no verdict
// to be missing; saying the check never answered would call a finished turn broken.
func (s *Server) recordParts(page map[string]any, found tasks.Record, dir task.Dir, manifest *record.Manifest, noCheck bool) {
	if manifest != nil {
		page["Build"] = manifest.HarnessVersion
		if capped := manifest.StoppedByCap; capped != nil {
			page["StoppedByCap"] = capStopView{Cap: capped.Cap, Reason: capped.Reason}
		}
	}
	if patch, ok := readPatch(dir, found.ID); ok {
		page["Patch"] = patchFilesView{Empty: patch.Empty(), Files: patch.Files, cutView: cutOf(patch.excerpt)}
	}
	if !found.State.Done() {
		page["Live"] = true
		return
	}
	if transcript, ok := readExcerpt(dir, found.ID, "transcript.log", drawn); ok {
		page["Transcript"] = transcriptView{Empty: transcript.Empty(), HTML: string(markdown(transcript.Text)), cutView: cutOf(transcript)}
	}
	page["Home"] = homeFullOf(dir)
	if !noCheck {
		page["Check"] = checkPageOf(readCheck(found, dir, manifest))
	}
}

func cutOf(part excerpt) cutView {
	return cutView{Cut: part.Cut, Name: part.Name, Raw: part.Raw, Whole: part.Whole(), Shown: part.Shown()}
}

func callsViewOf(made *calls) *callsView {
	if made == nil {
		return nil
	}
	return &callsView{InputTokens: compact(made.Tally.InputTokens), OutputTokens: compact(made.Tally.OutputTokens),
		Denied: len(made.Denied), Sites: made.Sites()}
}

func checkPageOf(check checkView) checkPageView {
	shown := checkPageView{Command: check.Command, Explains: check.Explains}
	if check.Verdict != nil {
		shown.Verdict = &verdictView{Passed: check.Verdict.Passed, Code: check.Verdict.Code}
	}
	if check.Log != nil {
		shown.Log = &checkLogView{Text: check.Log.Text, cutView: cutOf(*check.Log)}
	}
	return shown
}

func proofViewOf(held *proof) *proofView {
	if held == nil {
		return nil
	}
	return &proofView{Holds: held.Holds(), Signed: held.Signed, Files: held.Files, Wrong: held.Wrong, Signer: held.Signer,
		Unchecked: held.Unchecked, Key: held.Key, Bundle: held.Bundle, Command: held.Command}
}
