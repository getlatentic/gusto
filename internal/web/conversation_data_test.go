package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/roomlive"
)

// conversationJSON is a conversation page's data, as the React page reads it from /app/api/instances/{id}/threads/{thread}.
type conversationJSON struct {
	Instance          instanceView
	Thread            threadView
	Named, Path, Me   string
	Agent             agentView
	DirectAgent       *directAgentView
	Turns             []turnView
	Starters          []string
	Several           bool
	Models            []option
	Sharing, Owns     bool
	Apps              []appLinkView
	Starting, Waiting bool
	Latest, Holding   string
	Hint              string
	Mentions          []mentionable
	Names             []string
	Stream            *liveStream
	Version           string
	Headcount         int
	Settings          roomSettings
	Members           *membersView
	Guidelines        string
	MaxGuidelines     int
	Refused           string
	AddRefused        string
	GuidelinesRefused string
	Unsent            string
	AttachMost        int
	AttachBytes       int64
}

// turnsJSON is a conversation's turns as /app/api/.../turns gives them.
type turnsJSON struct {
	Turns    []turnView
	Version  string
	Settings string
}

// conversation is the data of the conversation at path, a thread's address with or without a query.
func (s *site) conversation(t *testing.T, path string) conversationJSON {
	t.Helper()
	var page conversationJSON
	s.data(t, path, &page)
	return page
}

// turnsAt is the turns snapshot of the conversation at path, with the answer's headers.
func (s *site) turnsAt(t *testing.T, path string) (turnsJSON, http.Header) {
	t.Helper()
	answer := s.ask(t, "GET", "/app/api"+path+"/turns", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("the turns of %s answered %d: %s", path, answer.Code, answer.Body.String())
	}
	var turns turnsJSON
	if err := json.Unmarshal(answer.Body.Bytes(), &turns); err != nil {
		t.Fatalf("the turns of %s are not JSON: %v", path, err)
	}
	return turns, answer.Header()
}

// turnIn is the turn with the id, and whether the conversation has it.
func (c conversationJSON) turnIn(id string) (turnView, bool) {
	for _, each := range c.Turns {
		if each.ID == id {
			return each, true
		}
	}
	return turnView{}, false
}

// shows says whether any turn's message or reply holds the text.
func (c conversationJSON) shows(text string) bool {
	for _, each := range c.Turns {
		if strings.Contains(each.Said, text) || strings.Contains(each.ReplyHTML, text) {
			return true
		}
	}
	return false
}

// turnOf is the turn a live frame carries, or nothing for a frame without one.
func turnOf(t *testing.T, frame roomlive.Frame) turnView {
	t.Helper()
	var turn turnView
	if len(frame.Turn) == 0 {
		return turn
	}
	if err := json.Unmarshal(frame.Turn, &turn); err != nil {
		t.Fatalf("the frame's turn is not JSON: %v", err)
	}
	return turn
}

// roomFilesJSON is a room's files as /app/api/rooms/{id}/files gives them.
type roomFilesJSON struct {
	Room           struct{ ID, Title string }
	From           string
	ByAgents       []roomFileDay
	ByPeople       []roomFileDay
	Agents, People int
	Cut, Archived  bool
}

func (s *site) roomFilesOf(t *testing.T, id string) roomFilesJSON {
	t.Helper()
	var files roomFilesJSON
	s.data(t, "/rooms/"+id+"/files", &files)
	return files
}

// names is every file's name in the days, in order.
func names(days []roomFileDay) []string {
	var all []string
	for _, day := range days {
		for _, file := range day.Files {
			all = append(all, file.Name)
		}
	}
	return all
}

// lastTurn is the newest turn of the conversation at path.
func (s *site) lastTurn(t *testing.T, path string) turnView {
	t.Helper()
	page := s.conversation(t, path)
	if len(page.Turns) == 0 {
		t.Fatalf("the conversation at %s has no turns", path)
	}
	return page.Turns[len(page.Turns)-1]
}

// working is how many of the conversation's turns are running, and what each turn that waits says it waits for.
func (c conversationJSON) working() (int, []string) {
	running, waits := 0, []string{}
	for _, each := range c.Turns {
		switch {
		case each.Kind != "turn" || each.Done:
		case each.Holding != "":
			waits = append(waits, each.Holding)
		default:
			running++
		}
	}
	return running, waits
}
