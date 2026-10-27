package cloud

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// fakeOrgDoor records the forms an organization's server is sent, and answers as it does.
func fakeOrgDoor(t *testing.T) (*orgAs, *[]string) {
	t.Helper()
	var guard sync.Mutex
	var sent []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("gusto"); err != nil {
			http.Error(w, "no cookie", http.StatusForbidden)
			return
		}
		_ = r.ParseForm()
		guard.Lock()
		sent = append(sent, r.URL.Path+" "+r.PostForm.Encode())
		guard.Unlock()
		switch {
		case r.URL.Path == "/members/agents":
			http.Redirect(w, r, "/members/agents/id-"+r.PostFormValue("name"), http.StatusSeeOther)
		case r.URL.Path == "/rooms":
			http.Redirect(w, r, "/instances/room-1/threads/1?add=1", http.StatusSeeOther)
		default:
			http.Redirect(w, r, "/instances/room-1/threads/1?members=1", http.StatusSeeOther)
		}
	}))
	t.Cleanup(server.Close)
	return &orgAs{base: server.URL, cookie: &http.Cookie{Name: "gusto", Value: "x"}, client: &http.Client{CheckRedirect: stayHere}}, &sent
}

func TestTheDemoRoomHoldsBothAgents(t *testing.T) {
	as, sent := fakeOrgDoor(t)
	room, err := makeDemoRoom(as)
	if err != nil || room != "room-1" {
		t.Fatalf("room %q, %v", room, err)
	}
	all := strings.Join(*sent, "\n")
	for _, want := range []string{"harness=opencode", "name=Scout", "name=Draft", "name=Launch+plan", "/rooms/room-1/members/add agent=id-Scout&agent=id-Draft"} {
		if !strings.Contains(all, want) {
			t.Errorf("the server was never sent %q:\n%s", want, all)
		}
	}
}

func TestARefusalOfTheServerIsAnError(t *testing.T) {
	as := &orgAs{cookie: &http.Cookie{Name: "gusto", Value: "x"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/members?new=1&refused=nope", http.StatusSeeOther)
	}))
	defer server.Close()
	as.base, as.client = server.URL, &http.Client{CheckRedirect: stayHere}
	if _, err := as.post("/members/agents", url.Values{"name": {"Scout"}}); err == nil {
		t.Fatal("a refused agent was taken for a made one")
	}
}
