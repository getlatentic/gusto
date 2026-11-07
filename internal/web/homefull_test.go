package web

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/task"
)

// A turn the kept home's size refused says so where it is shown, with the way out, instead of only the error the agent left.
func TestATurnRefusedForAFullHomeSaysSoAndLinksToTheReset(t *testing.T) {
	testWork = func(ctx context.Context, dir task.Dir) (int, error) {
		os.MkdirAll(dir.Out(), 0o755)
		os.WriteFile(dir.OutFile("home.json"), []byte(`{"outcome":"refused","used":6442450944,"limit":5368709120}`), 0o644)
		os.WriteFile(dir.OutFile("stderr.log"), []byte("gusto: the kept home is over its size limit\n"), 0o644)
		return 1, nil
	}
	t.Cleanup(func() { testWork = nil })
	here := withTokens(t)
	ada := here.as(t, alice)
	_, thread := started2(t, here, alice)
	_, said := here.say("/api"+thread+"/say", signInADevice(t, here, alice), "one more", "go")
	id := said["job"].(string)
	turnDone(t, here, id)

	page := ada.ask(t, "GET", thread, nil).Body.String()
	for _, want := range []string{"Your agents' home is full", "6.0 GiB of the 5.0 GiB", "was refused", `href="/connections"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the conversation does not say %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "It stopped before it finished") {
		t.Error("the conversation shows the agent's error as well as the full home")
	}

	admin := *here.site
	admin.signIn(t)
	task := admin.ask(t, "GET", "/jobs/"+id, nil).Body.String()
	if !strings.Contains(task, "The home their agents keep is full") || strings.Contains(task, `href="/connections"`) {
		t.Errorf("the task page, read by somebody else, does not say whose home is full:\n%s", task)
	}
}

// A turn that failed for another reason says nothing of the home, and a note that is not one is ignored.
func TestATurnThatFailedForAnotherReasonSaysNothingOfTheHome(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"no note":      {"stderr.log": "boom\n"},
		"not a note":   {"stderr.log": "boom\n", "home.json": "not json"},
		"no limit set": {"stderr.log": "boom\n", "home.json": `{"outcome":"refused"}`},
	} {
		if homeFullOf(failedTurn(t, files)) != nil {
			t.Errorf("%s: the turn is said to have a full home", name)
		}
	}
}
