package web

import (
	"net/http"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/getlatentic/gusto/internal/job"
)

// liveItem is one thing in a running turn, in the order it happened: words the agent said, or a step it took.
type liveItem struct {
	// Kind is "said" or "step".
	Kind   string `json:"kind"`
	Text   string `json:"text,omitempty"`
	What   string `json:"what,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// liveTurn is what a turn that is running has done so far, for the page to show while it waits: all of it, as it came, with the words
// between the steps where they were said.
type liveTurn struct {
	Items []liveItem `json:"items"`
	Steps int        `json:"steps"`
	// Cut is whether the start of a very long turn is left out.
	Cut bool `json:"cut,omitempty"`
}

// liveRead is the most of the transcript a page is given; a turn is not the place to read a megabyte.
const liveRead = 128 * 1024

// turnLive answers what the thread's running turn has done so far.
func (s *Server) turnLive(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	thread, found := s.instances.Thread(held.ID, r.PathValue("thread"))
	if !found || len(thread.Turns) == 0 {
		http.NotFound(w, r)
		return
	}
	record, found := s.index.Get(thread.Turns[len(thread.Turns)-1])
	if !found {
		http.NotFound(w, r)
		return
	}
	answerJSON(w, http.StatusOK, timelineOf(record.Dir(s.index.Root())))
}

// timelineOf reads what a turn said and ran into one list in the order it happened. A step says how much had been said when it ran;
// a run from before that was kept has no such place, and is given as its steps and then its words.
func timelineOf(dir job.Dir) liveTurn {
	said, cut := transcriptOf(dir)
	ran := ranBy(dir)
	live := liveTurn{Items: []liveItem{}, Steps: len(ran), Cut: cut.dropped > 0}
	placed := true
	for _, each := range ran {
		placed = placed && each.At != nil
	}
	if !placed {
		for _, each := range ran {
			shown := describe(each)
			live.Items = append(live.Items, liveItem{Kind: "step", What: shown.What, Detail: shown.Detail})
		}
		if text := strings.TrimSpace(said); text != "" {
			live.Items = append(live.Items, liveItem{Kind: "said", Text: text})
		}
		return live
	}
	at := 0
	for _, each := range ran {
		end := min(max(int(*each.At)-cut.dropped, at), len(said))
		for end > at && end < len(said) && !utf8.RuneStart(said[end]) {
			end++
		}
		if text := strings.TrimSpace(said[at:end]); text != "" {
			live.Items = append(live.Items, liveItem{Kind: "said", Text: text})
		}
		at = end
		shown := describe(each)
		live.Items = append(live.Items, liveItem{Kind: "step", What: shown.What, Detail: shown.Detail})
	}
	if text := strings.TrimSpace(said[at:]); text != "" {
		live.Items = append(live.Items, liveItem{Kind: "said", Text: text})
	}
	return live
}

// dropped is how many bytes of the start of the transcript were left out.
type dropped struct{ dropped int }

// transcriptOf is the transcript as far as a page is given it. When it is longer than that its start is left out, at a place where a
// word begins, so that no word is shown cut in two.
func transcriptOf(dir job.Dir) (string, dropped) {
	if !dir.PlainOut("transcript.log") {
		return "", dropped{}
	}
	text, err := os.ReadFile(dir.OutFile("transcript.log"))
	if err != nil {
		return "", dropped{}
	}
	skip := 0
	if len(text) > liveRead {
		skip = len(text) - liveRead
		for skip < len(text) && text[skip] != ' ' && text[skip] != '\n' {
			skip++
		}
	}
	return strings.ToValidUTF8(string(text[skip:]), ""), dropped{dropped: skip}
}
