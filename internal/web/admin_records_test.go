package web

import (
	"os"
	"strings"
	"testing"
)

func TestTheRecordsTopLineSaysHowManyAndWhichDoNotHold(t *testing.T) {
	for _, each := range []struct {
		said, want string
	}{
		{eventsSaid(0, 0), "no events yet"},
		{eventsSaid(1, 1), "1 event, all signed"},
		{eventsSaid(133, 133), "133 events, all signed"},
		{eventsSaid(133, 0), "133 events, none signed"},
		{eventsSaid(133, 120), "133 events, 120 signed"},
		{runsSaid(runChecks{}), "no task has finished yet"},
		{runsSaid(runChecks{Verified: 1}), "the last task checks out"},
		{runsSaid(runChecks{Failing: 1}), "the last task does not check out"},
		{runsSaid(runChecks{Verified: 20, Unsigned: 5}), "the last 25 tasks check out"},
		{runsSaid(runChecks{Verified: 24, Failing: 1}), "1 of the last 25 tasks does not check out"},
		{runsSaid(runChecks{Verified: 22, Failing: 3}), "3 of the last 25 tasks do not check out"},
	} {
		if each.said != each.want {
			t.Errorf("said %q, not %q", each.said, each.want)
		}
	}
}

// A record whose chain is cut says so at the top of every tab, in the colour for trouble, and offers no head to keep.
func TestARecordThatDoesNotHoldSaysSoOnEveryTab(t *testing.T) {
	here, log := recording(t)
	here.signIn(t)
	file, err := os.OpenFile(log.Path(), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("{\"seq\":99}\n"); err != nil {
		t.Fatal(err)
	}
	file.Close()
	for _, path := range []string{"/admin/records", "/admin/records?tab=checks"} {
		page := here.ask(t, "GET", path, nil).Body.String()
		if !strings.Contains(page, `text-bad">Broken</strong>`) || !strings.Contains(page, "the activity log does not hold together") {
			t.Errorf("%s does not say the record is broken:\n%s", path, page)
		}
		if strings.Contains(page, "gusto changes verify -head") {
			t.Errorf("%s offers a head for a record that does not hold", path)
		}
	}
}
