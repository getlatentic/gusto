package cloud

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

var demoSet = Settings{Demo: DemoSettings{Length: 30 * time.Minute, Extend: 10 * time.Minute, MaxOrgs: 3, Patience: 10 * time.Minute, BookURL: "https://cal.example/haas"}}

// clocked is a store whose clock a test moves.
func clocked(t *testing.T, set Settings) (*Store, *time.Time) {
	t.Helper()
	s := open(t, set)
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, &now
}

func TestAVisitorStartsADemoOrganisationOnTheDemoQuotaAndIsKeptAsALead(t *testing.T) {
	s, _ := clocked(t, demoSet)
	person, org, err := s.StartDemo("Ada Lovelace", "Ada@Acme.com", "Acme Research")
	if err != nil {
		t.Fatal(err)
	}
	if person.Role != Admin || person.Email != "ada@acme.com" || person.Org != org.Slug || org.Demo == nil || org.Demo.Name != "Ada Lovelace" {
		t.Fatalf("%+v %+v", person, org)
	}
	if !strings.HasPrefix(org.Slug, "demo-acme-research-") || ValidSlug(org.Slug) != nil {
		t.Errorf("an address for a demo: %q", org.Slug)
	}
	if got, _ := s.Quota(org.Slug); got.Computers != 1 || got.Members != 1 || got.Jobs == 0 {
		t.Errorf("a demo runs what a demo may: %+v", got)
	}
	lead, found := s.LeadOf(org.Slug)
	if !found || lead.Email != "ada@acme.com" || lead.Company != "Acme Research" || lead.Name != "Ada Lovelace" || !lead.Ended.IsZero() {
		t.Errorf("%+v", lead)
	}
	if _, _, err := s.StartDemo("Ada Again", "ada@acme.com", "Acme"); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("one demo for an address at a time: %v", err)
	}
	_, other, err := s.StartDemo("Bob", "bob@acme.com", "Acme Research")
	if err != nil || other.Slug == org.Slug {
		t.Errorf("two people from one company have a demo each: %q %q %v", other.Slug, org.Slug, err)
	}
}

func TestAVisitorWhoLeavesOutWhoTheyAreOrHaveAnUnusableAddressStartsNothing(t *testing.T) {
	s, _ := clocked(t, demoSet)
	for name, in := range map[string][3]string{
		"no name":         {"", "ada@acme.com", "Acme"},
		"no company":      {"Ada", "ada@acme.com", " "},
		"not an address":  {"Ada", "ada", "Acme"},
		"a throwaway":     {"Ada", "ada@mailinator.com", "Acme"},
		"a free provider": {"Ada", "ada@gmail.com", "Acme"},
		"a long name":     {strings.Repeat("a", 81), "ada@acme.com", "Acme"},
	} {
		if _, _, err := s.StartDemo(in[0], in[1], in[2]); err == nil {
			t.Errorf("%s started a demo", name)
		}
	}
	if orgs, _ := s.Orgs(); len(orgs) != 0 {
		t.Errorf("a refusal left an organisation: %+v", orgs)
	}
	if _, _, err := open(t, Settings{}).StartDemo("Ada", "ada@acme.com", "Acme"); err == nil {
		t.Error("a host that offers no demo made one")
	}
}

func TestADemosTimeStartsWhenItsServerIsReadyAndNotWhenItWasAskedFor(t *testing.T) {
	s, now := clocked(t, demoSet)
	_, org, _ := s.StartDemo("Ada", "ada@acme.com", "Acme")
	*now = now.Add(5 * time.Minute)
	if _, left, found := s.DemoLeft(org.Slug); !found || left != 30*time.Minute {
		t.Errorf("a demo that is not ready has all its time: %s %v", left, found)
	}
	demo, err := s.DemoReady(org.Slug)
	if err != nil || !demo.Expires.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("%+v %v", demo, err)
	}
	*now = now.Add(10 * time.Minute)
	if again, _ := s.DemoReady(org.Slug); !again.Expires.Equal(demo.Expires) {
		t.Errorf("being ready again does not start the time again: %v", again.Expires)
	}
	if _, left, _ := s.DemoLeft(org.Slug); left != 20*time.Minute {
		t.Errorf("%s", left)
	}
	*now = now.Add(time.Hour)
	if _, left, _ := s.DemoLeft(org.Slug); left != 0 {
		t.Errorf("a demo has no negative time: %s", left)
	}
	if lead, _ := s.LeadOf(org.Slug); lead.Ready.IsZero() {
		t.Error("the lead says when the demo was ready")
	}
	if _, err := s.DemoReady("acme-com"); err == nil {
		t.Error("an organisation that is not a demo was started")
	}
}

func TestADemoIsExtendedOnceAndOnlyWhileItHasTime(t *testing.T) {
	s, now := clocked(t, demoSet)
	_, org, _ := s.StartDemo("Ada", "ada@acme.com", "Acme")
	if _, err := s.ExtendDemo(org.Slug); !errors.Is(err, ErrNoExtension) {
		t.Errorf("a demo that is not ready is not extended: %v", err)
	}
	ready, _ := s.DemoReady(org.Slug)
	*now = now.Add(25 * time.Minute)
	extended, err := s.ExtendDemo(org.Slug)
	if err != nil || !extended.Expires.Equal(ready.Expires.Add(10*time.Minute)) || !extended.Extended {
		t.Fatalf("%+v %v", extended, err)
	}
	if _, err := s.ExtendDemo(org.Slug); !errors.Is(err, ErrNoExtension) {
		t.Errorf("one extension: %v", err)
	}
	if lead, _ := s.LeadOf(org.Slug); !lead.Extended {
		t.Error("the lead says it was extended")
	}
	late, lateNow := clocked(t, demoSet)
	_, other, _ := late.StartDemo("Bob", "bob@acme.com", "Acme")
	late.DemoReady(other.Slug)
	*lateNow = lateNow.Add(time.Hour)
	if _, err := late.ExtendDemo(other.Slug); !errors.Is(err, ErrNoExtension) {
		t.Errorf("a demo that has ended is not brought back: %v", err)
	}
}

func TestADemoIsDueWhenItsTimeIsUpOrItWasNeverReadyAndNoOtherOrganisationIs(t *testing.T) {
	s, now := clocked(t, demoSet)
	_, running, _ := s.StartDemo("Ada", "ada@acme.com", "Acme")
	_, waiting, _ := s.StartDemo("Bob", "bob@globex.com", "Globex")
	_, real, _ := s.SignUp(Signup{Email: "cy@initech.com", OrgName: "Initech"})
	s.DemoReady(running.Slug)
	if due, _ := s.DemosDue(); len(due) != 0 {
		t.Fatalf("nothing is due yet: %v", due)
	}
	*now = now.Add(11 * time.Minute)
	due, _ := s.DemosDue()
	if len(due) != 1 || due[waiting.Slug] != "it was never ready" {
		t.Errorf("a demo that waited too long for a server: %v", due)
	}
	*now = now.Add(time.Hour)
	due, _ = s.DemosDue()
	if due[running.Slug] != "its time was up" || len(due) != 2 {
		t.Errorf("%v", due)
	}
	if _, found := due[real.Slug]; found {
		t.Error("an organisation that is not a demo is due to be deleted")
	}
}

func TestEndingADemoDeletesItsOrganisationAndPeopleKeepsTheLeadAndFreesTheAddress(t *testing.T) {
	s, now := clocked(t, demoSet)
	person, org, _ := s.StartDemo("Ada", "ada@acme.com", "Acme")
	s.DemoReady(org.Slug)
	*now = now.Add(31 * time.Minute)
	if err := s.EndDemo(org.Slug, "its time was up"); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := s.Org(org.Slug); found {
		t.Error("the organisation is still there")
	}
	if _, err := os.Stat(s.OrgDir(org.Slug)); err == nil {
		t.Error("its directory is still there")
	}
	if _, found, _ := s.Person(person.Email); found {
		t.Error("the person is still there")
	}
	lead, found := s.LeadOf(org.Slug)
	if !found || lead.Reason != "its time was up" || !lead.Ended.Equal(*now) || lead.Email != "ada@acme.com" {
		t.Errorf("%+v", lead)
	}
	if _, _, err := s.StartDemo("Ada", "ada@acme.com", "Acme"); err != nil {
		t.Errorf("the address can start another: %v", err)
	}
}

func TestEndingAnOrganisationThatIsNotADemoIsRefusedAndDeletesNothing(t *testing.T) {
	s, _ := clocked(t, demoSet)
	person, org, _ := s.SignUp(Signup{Email: "cy@initech.com", OrgName: "Initech"})
	if err := s.EndDemo(org.Slug, "a mistake"); err == nil {
		t.Fatal("a real organisation was ended as a demo")
	}
	if _, found, _ := s.Org(org.Slug); !found {
		t.Error("the organisation was deleted")
	}
	if _, found, _ := s.Person(person.Email); !found {
		t.Error("the person was deleted")
	}
	if err := s.EndDemo("nobody", "x"); err == nil {
		t.Error("an organisation that is not there was ended")
	}
}

func TestOnlyAsManyDemosRunAtOnceAsTheHostAllows(t *testing.T) {
	set := demoSet
	set.Demo.MaxOrgs = 2
	s, _ := clocked(t, set)
	var first Org
	for i, email := range []string{"a@one.com", "b@two.com"} {
		_, org, err := s.StartDemo("Person", email, "Company")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = org
		}
	}
	if _, _, err := s.StartDemo("Person", "c@three.com", "Company"); !errors.Is(err, ErrDemoFull) {
		t.Errorf("%v", err)
	}
	s.EndDemo(first.Slug, "its time was up")
	if _, _, err := s.StartDemo("Person", "c@three.com", "Company"); err != nil {
		t.Errorf("a demo that ended makes room: %v", err)
	}
}
