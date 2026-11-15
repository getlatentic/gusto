package web

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/retention"
)

// recordsJSON is the Records page's data: the record's health on every tab, and the chosen tab's own part.
type recordsJSON struct {
	Tabs     []tab
	Tab      string
	Refused  string
	Health   health
	Logged   bool
	Activity *activityJSON
	Checks   *runChecks
	Keeping  *keepingJSON
}

type activityJSON struct {
	Rows       []row
	LeftOut    int
	WithServer bool
}

type keepingJSON struct {
	Used, WorkingFolders  string
	Jobs                  int
	Policy                []retention.Row
	Removes               bool
	HeldTasks, HeldPeople map[string]retention.Hold
	Oldest, OldestAt      string
	Last                  *retention.Report
	LastAt, LastFreed     string
	Upcoming              []retention.Action
	UpcomingFreed         string
}

// recordsPage is the Records page's data at the query given.
func recordsPage(t *testing.T, here dataReader, query string) recordsJSON {
	t.Helper()
	var page recordsJSON
	here.data(t, recordsPath+query, &page)
	return page
}

// said is every sentence the activity tab lists, one to a line.
func (a *activityJSON) said() string {
	if a == nil {
		return ""
	}
	var all []string
	for _, each := range a.Rows {
		all = append(all, each.Said)
	}
	return strings.Join(all, "\n")
}

// tabHrefs is the address of each tab Records offers.
func (p recordsJSON) tabHrefs() string {
	var all []string
	for _, each := range p.Tabs {
		all = append(all, each.Href)
	}
	return strings.Join(all, " ")
}

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
	for _, query := range []string{"", "?tab=checks"} {
		got := recordsPage(t, here, query).Health
		if !got.Broken || !got.LogBad || !strings.Contains(got.Log, "the activity log does not hold together") {
			t.Errorf("Records%s does not say the record is broken: %+v", query, got)
		}
	}
}

// Each tab gives its own part and no other's, with the record's health on all of them.
func TestEachTabOfRecordsGivesItsOwnPart(t *testing.T) {
	admin := everything(t).as(t, boss)
	parts := func(query string) string {
		var page map[string]json.RawMessage
		admin.data(t, recordsPath+query, &page)
		var have []string
		for _, part := range []string{"Activity", "Checks", "Keeping"} {
			if _, found := page[part]; found {
				have = append(have, part)
			}
		}
		return strings.Join(have, " ")
	}
	for query, want := range map[string]string{"": "Activity", "?tab=checks": "Checks", "?tab=keeping": "Keeping"} {
		if got := parts(query); got != want {
			t.Errorf("Records%s gives %q, not %q", query, got, want)
		}
		page := recordsPage(t, admin, query)
		if page.Health.Broken || !page.Logged {
			t.Errorf("Records%s does not give the record as intact: %+v", query, page.Health)
		}
		if got := page.tabHrefs(); got != "/admin/records?tab=activity /admin/records?tab=checks /admin/records?tab=keeping" {
			t.Errorf("Records%s offers the tabs %s", query, got)
		}
	}
}
