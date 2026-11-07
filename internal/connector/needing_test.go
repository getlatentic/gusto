package connector

import (
	"context"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/connector/connectortest"
)

// An admin sees how many sign-ins to each app need making again, in every workspace, and not the ones that work.
func TestTheAdminCanCountTheSignInsThatNeedMakingAgain(t *testing.T) {
	a := connectortest.New(t)
	c := connectors(t, "https://acme.gusto.test")
	a.GivesNoRefresh()
	a.Lasts(time.Hour)
	entry := Entry{ID: "tasks", Name: "Tasks", URL: a.MCP()}
	c.config.Apps = []Entry{entry}
	signInTo := func(person, space string) {
		t.Helper()
		state, code := a.Approve(t, mustBegin2(t, c, person, space, entry))
		if _, _, err := c.Finish(context.Background(), person, state, code); err != nil {
			t.Fatal(err)
		}
	}
	signInTo(ada, "")
	signInTo("bob@acme.com", "")
	signInTo(ada, "second")
	if got := c.NeedingSignIn()["tasks"]; got != 0 {
		t.Fatalf("%d sign-ins need making again while all work", got)
	}
	c.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if got := c.NeedingSignIn()["tasks"]; got != 3 {
		t.Errorf("%d sign-ins need making again, not the 3 that ran out", got)
	}
}

func mustBegin2(t *testing.T, c *Connectors, person, space string, entry Entry) string {
	t.Helper()
	address, err := c.Begin(context.Background(), person, space, entry)
	if err != nil {
		t.Fatal(err)
	}
	return address
}
