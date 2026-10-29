package cloud

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/entry"
)

// A demo opens on a room with two agents in it, of two agent types, so a visitor sees a team of several vendors' agents at work and
// has somewhere to bring a colleague. Each type finishes a task on the demo's models (docs/MATRIX.md) in seconds in a room, and the
// agents take the model the server offers first.
const (
	demoRoomName = "Launch plan"
	demoRoomFile = "demo-room"
)

var demoCrew = []struct{ Name, Specialty, Harness string }{
	{"Scout", "Finds things out and reports what it found", "opencode"},
	{"Draft", "Writes the first version of a document", "pi"},
}

// orgAs is a request-maker that speaks to an organization's server as one of its people, through the entry key only the front door
// and that server share.
type orgAs struct {
	base   string
	cookie *http.Cookie
	client *http.Client
}

// stayHere keeps a client from following the redirect it was given, because where the server sent the person is the answer.
func stayHere(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// signedInAs enters an organization's server as a person of it.
func (c *Controller) signedInAs(person Person) (*orgAs, error) {
	address, state := c.Route(person.Org)
	if state != Ready {
		return nil, fmt.Errorf("the server is %s", state)
	}
	key, err := c.EntryKey(person.Org)
	if err != nil {
		return nil, err
	}
	token, err := entry.Mint(key, person.Org, person.Email, string(person.Role), time.Now())
	if err != nil {
		return nil, err
	}
	as := &orgAs{base: "http://" + address, client: &http.Client{
		Timeout:       20 * time.Second,
		CheckRedirect: stayHere,
	}}
	response, err := as.client.Get(as.base + "/auth/entry?t=" + url.QueryEscape(token))
	if err != nil {
		return nil, err
	}
	response.Body.Close()
	if len(response.Cookies()) == 0 {
		return nil, fmt.Errorf("the server did not sign %s in (%d)", person.Email, response.StatusCode)
	}
	as.cookie = response.Cookies()[0]
	return as, nil
}

// post sends a form and returns where the server sent the person on to. A refusal is an error carrying the server's words.
func (o *orgAs) post(path string, form url.Values) (string, error) {
	request, err := http.NewRequest(http.MethodPost, o.base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(o.cookie)
	response, err := o.client.Do(request)
	if err != nil {
		return "", err
	}
	response.Body.Close()
	where := response.Header.Get("Location")
	if response.StatusCode != http.StatusSeeOther || strings.Contains(where, "refused=") {
		return "", fmt.Errorf("POST %s gave %d to %q", path, response.StatusCode, where)
	}
	return where, nil
}

// roomIn is the id of the room in a path such as /instances/room-x/threads/1?add=1.
func roomIn(where string) string {
	id, _, _ := strings.Cut(strings.TrimPrefix(where, "/instances/"), "/threads/")
	return id
}

// makeDemoRoom makes the demo's agents and the room they are in, and returns the room's id.
func makeDemoRoom(as *orgAs) (string, error) {
	agents := make([]string, 0, len(demoCrew))
	for _, each := range demoCrew {
		where, err := as.post("/members/agents", url.Values{"name": {each.Name}, "specialty": {each.Specialty}, "harness": {each.Harness}})
		if err != nil {
			return "", err
		}
		agents = append(agents, strings.TrimPrefix(where, "/members/agents/"))
	}
	where, err := as.post("/rooms", url.Values{"name": {demoRoomName}})
	if err != nil {
		return "", err
	}
	room := roomIn(where)
	if _, err := as.post("/rooms/"+room+"/members/add", url.Values{"agent": agents}); err != nil {
		return "", err
	}
	return room, nil
}

// seedDemoRoom makes the demo's room once, when its server first answers. A demo whose room could not be made still works, as a
// chat, so a failure is told to the operator and not to the visitor.
func (s *Server) seedDemoRoom(person Person) {
	file := filepath.Join(s.Store.OrgDir(person.Org), demoRoomFile)
	if _, err := os.Stat(file); err == nil {
		return
	}
	as, err := s.Controller.signedInAs(person)
	if err == nil {
		var room string
		if room, err = makeDemoRoom(as); err == nil {
			err = os.WriteFile(file, []byte(room), 0o644)
		}
	}
	if err != nil {
		slog.Warn("a demo's room was not made", "org", person.Org, "error", err)
	}
}

// addToDemoRoom puts a colleague in the demo's room, so it is the first thing they see.
func (s *Server) addToDemoRoom(admin Person, colleague string) {
	room, err := os.ReadFile(filepath.Join(s.Store.OrgDir(admin.Org), demoRoomFile))
	if err != nil {
		return
	}
	as, err := s.Controller.signedInAs(admin)
	if err == nil {
		_, err = as.post("/rooms/"+string(room)+"/members", url.Values{"who": {colleague}})
	}
	if err != nil {
		slog.Warn("a colleague was not put in the demo's room", "org", admin.Org, "error", err)
	}
}
