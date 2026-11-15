package connector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/connector/connectortest"
)

const ada = "ada@acme.com"

func signedIn(t *testing.T, a *connectortest.App, c *Connectors) {
	t.Helper()
	state, code := a.Approve(t, mustBegin(t, c, ada, Entry{ID: "tasks", Name: "Tasks", URL: a.MCP()}))
	if _, _, err := c.Finish(context.Background(), ada, state, code); err != nil {
		t.Fatal(err)
	}
}

// A sign-in whose access token ran out, with no refresh token to renew it, can never work again, so it is not connected.
func TestASignInThatRanOutWithNothingToRenewItIsNotConnected(t *testing.T) {
	a := connectortest.New(t)
	c := connectors(t, "https://acme.gusto.test")
	a.GivesNoRefresh()
	signedIn(t, a, c)
	if standing, _ := c.Standing(ada, "", "tasks"); standing != SignedIn {
		t.Fatalf("a fresh sign-in is %v", standing)
	}
	c.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, connected := c.Connected(ada, "", "tasks"); connected {
		t.Error("a sign-in that ran out is connected")
	}
	if standing, _ := c.Standing(ada, "", "tasks"); standing != NeedsSignIn {
		t.Errorf("a sign-in that ran out is %v, not one to make again", standing)
	}
	if standing, _ := c.Standing("bob@acme.com", "", "tasks"); standing != NotSignedIn {
		t.Errorf("a person who never signed in is %v", standing)
	}
}

// The app rotates refresh tokens, so once it answered a renewal the token on disk is dead. When the new ones cannot be written
// they are kept until they can be, and the sign-in goes on working.
func TestRenewedTokensThatCannotBeWrittenAreKeptUntilTheyCan(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("a read-only folder stops nobody who is root")
	}
	a := connectortest.New(t)
	a.Rotates()
	a.Lasts(30 * time.Second)
	c := connectors(t, "https://acme.gusto.test")
	signedIn(t, a, c)
	folder := filepath.Dir(filepath.Join(c.vault.dir, tokenName(ada, "", "tasks")))
	if err := os.Chmod(folder, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(folder, 0o700) })

	if _, err := c.Token(context.Background(), ada, "", "tasks"); err != nil {
		t.Fatalf("a renewal the app made was lost to a failed write: %v", err)
	}
	if _, connected := c.Connected(ada, "", "tasks"); !connected {
		t.Fatal("a sign-in that renewed shows as not connected")
	}
	if err := os.Chmod(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Token(context.Background(), ada, "", "tasks"); err != nil {
		t.Fatalf("the sign-in broke because its new refresh token was never kept: %v", err)
	}
	if len(c.unsaved) != 0 {
		t.Errorf("tokens still wait to be written: %d", len(c.unsaved))
	}
	if _, connected := c.Connected(ada, "", "tasks"); !connected {
		t.Error("the sign-in is not connected once it could be written")
	}
}

// Somebody who comes back with another person's state is turned away without ending the sign-in it names, and a sign-in that
// came back too late is used up.
func TestAnotherPersonsStateDoesNotEndASignIn(t *testing.T) {
	a := connectortest.New(t)
	c := connectors(t, "https://acme.gusto.test")
	state, code := a.Approve(t, mustBegin(t, c, ada, Entry{ID: "tasks", Name: "Tasks", URL: a.MCP()}))
	if _, _, err := c.Finish(context.Background(), "bob@acme.com", state, code); err == nil {
		t.Fatal("a sign-in begun by one person was finished by another")
	}
	if _, _, err := c.Finish(context.Background(), ada, state, code); err != nil {
		t.Fatalf("the person's own sign-in was ended by somebody else's attempt: %v", err)
	}

	late, code := a.Approve(t, mustBegin(t, c, ada, Entry{ID: "tasks", Name: "Tasks", URL: a.MCP()}))
	c.now = func() time.Time { return time.Now().Add(signInTime + time.Minute) }
	if _, _, err := c.Finish(context.Background(), ada, late, code); err == nil || !strings.Contains(err.Error(), "start it again") {
		t.Fatalf("a sign-in that came back too late was kept: %v", err)
	}
	if len(c.pending) != 0 {
		t.Errorf("a sign-in that came back too late is still waiting: %d", len(c.pending))
	}
}

// A lock is kept only while somebody holds it or waits for it, so the locks do not grow with the sign-ins ever made.
func TestSignInLocksAreForgottenWhenNobodyWantsThem(t *testing.T) {
	a := connectortest.New(t)
	c := connectors(t, "https://acme.gusto.test")
	signedIn(t, a, c)
	for i := range 20 {
		c.Token(context.Background(), ada, "", "tasks")
		c.SetForAgents(ada, "", "tasks", true)
		c.Disconnect(fmt.Sprintf("person%d@acme.com", i), "", "tasks")
	}
	c.mu.Lock()
	held := len(c.locks)
	c.mu.Unlock()
	if held != 0 {
		t.Errorf("%d locks are kept that nobody wants", held)
	}
}
