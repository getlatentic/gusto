package web

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/backend/container"
)

// connectionsHomes is a backend's kept homes, one for each person and workspace, that keeps what it was asked.
type connectionsHomes struct {
	mu     sync.Mutex
	uses   map[string]container.HomeUse
	resets []string
	busy   bool
}

func (h *connectionsHomes) KeepsHomes() bool { return true }

func (h *connectionsHomes) HomeUse(person, space string) (container.HomeUse, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	use, counted := h.uses[person+"/"+space]
	return use, counted
}

func (h *connectionsHomes) ResetHome(person, space string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.busy {
		return container.ErrHomeBusy
	}
	h.resets = append(h.resets, person+"/"+space)
	delete(h.uses, person+"/"+space)
	return nil
}

// A person sees how much of its limit their home takes, is told when it is full, and resets it themselves: only theirs, only in
// the workspace they are in, and not while their agents work in it.
func TestAPersonSeesTheirHomeTakesAndResetsIt(t *testing.T) {
	here, _, _ := connected(t)
	homes := &connectionsHomes{uses: map[string]container.HomeUse{
		alice + "/main": {Used: 1536 << 20, Limit: 5 << 30, At: time.Now().Add(-3 * time.Minute)},
		bob + "/main":   {Used: 6 << 30, Limit: 5 << 30, At: time.Now()},
	}}
	here.server.UseHomes(homes)
	ada, bea := here.as(t, alice), here.as(t, bob)

	page := ada.ask(t, "GET", "/connections", nil).Body.String()
	if !strings.Contains(page, "<strong>1.5 GiB</strong> of the <strong>5.0 GiB</strong>") || strings.Contains(page, "It is full") {
		t.Errorf("the person is not told how much of its limit their home takes:\n%s", page)
	}
	if page := bea.ask(t, "GET", "/connections", nil).Body.String(); !strings.Contains(page, "<strong>6.0 GiB</strong>") || !strings.Contains(page, "It is full, so a task you ask for is refused until you reset it.") {
		t.Errorf("a person whose home is over its limit is not told so:\n%s", page)
	}
	if page := here.ask(t, "GET", "/connections", nil).Body.String(); strings.Contains(page, "Your agents' home") {
		t.Error("the shared sign-in, which is nobody, is shown a home")
	}
	if got := here.ask(t, "POST", "/connections/home/reset", nil).Code; got != http.StatusNotFound {
		t.Errorf("the shared sign-in reset a home: %d", got)
	}

	homes.busy = true
	if got := bea.ask(t, "POST", "/connections/home/reset", nil); got.Code != http.StatusSeeOther || !strings.Contains(got.Header().Get("Location"), "refused") {
		t.Errorf("a home with a task running in it was reset: %d %q", got.Code, got.Header().Get("Location"))
	}
	homes.busy = false
	got := bea.ask(t, "POST", "/connections/home/reset", nil)
	if got.Code != http.StatusSeeOther || len(homes.resets) != 1 || homes.resets[0] != bob+"/main" {
		t.Fatalf("resetting went %d and reset %v, not only bob's", got.Code, homes.resets)
	}
	if page := bea.ask(t, "GET", got.Header().Get("Location"), nil).Body.String(); !strings.Contains(page, "Your agents' home is reset.") || !strings.Contains(page, "Nothing is counted yet.") {
		t.Errorf("the person is not told the home is reset:\n%s", page)
	}
	if page := ada.ask(t, "GET", "/connections", nil).Body.String(); !strings.Contains(page, "<strong>1.5 GiB</strong>") {
		t.Error("resetting bob's home touched alice's")
	}
}

// A server whose boxes keep no homes shows none and resets none.
func TestAServerThatKeepsNoHomesOffersNoReset(t *testing.T) {
	here, _, _ := connected(t)
	ada := here.as(t, alice)
	if page := ada.ask(t, "GET", "/connections", nil).Body.String(); strings.Contains(page, "Your agents' home") {
		t.Errorf("a server with no homes shows one:\n%s", page)
	}
	if got := ada.ask(t, "POST", "/connections/home/reset", nil).Code; got != http.StatusNotFound {
		t.Errorf("a server with no homes answered a reset: %d", got)
	}
}
