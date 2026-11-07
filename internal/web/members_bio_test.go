package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/instance"
)

func TestAPersonWritesTheirOwnBioAndAgentsReadItInTheirRooms(t *testing.T) {
	work := &roomWork{}
	site, log, id, thread := aRecordedRoom(t, work)
	mine, his := site.as(t, alice), site.as(t, bob)
	path := "/members/people/" + url.PathEscape(bob) + "/bio"

	if got := mine.ask(t, "POST", path, url.Values{"bio": {"Bob runs finance."}}); got.Code != http.StatusForbidden {
		t.Fatalf("somebody writing another person's bio answered %d", got.Code)
	}
	if got := his.ask(t, "POST", path, url.Values{"bio": {"I run finance.\nMonth-end is my busy week."}}); got.Code != http.StatusSeeOther || strings.Contains(got.Header().Get("Location"), "refused") {
		t.Fatalf("writing one's own bio answered %d to %s", got.Code, got.Header().Get("Location"))
	}

	if page := his.ask(t, "GET", "/members/people/"+url.PathEscape(bob), nil).Body.String(); !strings.Contains(page, `name="bio"`) || !strings.Contains(page, "Month-end is my busy week.") {
		t.Error("the person's own page has no form with their bio")
	}
	if page := mine.ask(t, "GET", "/members/people/"+url.PathEscape(bob), nil).Body.String(); strings.Contains(page, `name="bio"`) || !strings.Contains(page, "Month-end is my busy week.") {
		t.Error("another person sees a form, or does not see the bio")
	}

	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout who runs finance?"}})
	turns := work.waitTurns(t, site, 1)
	if brief := turns[0].Brief; !strings.Contains(brief, "What the people here say of themselves") || !strings.Contains(brief, "I run finance. Month-end is my busy week.") {
		t.Errorf("the agent was not told the bio: %q", brief)
	}

	if got := ofKind(written(t, log), changelog.PersonBio); len(got) != 1 || got[0].Who != bob || !strings.Contains(got[0].What["now"], "finance") {
		t.Errorf("the change record holds %+v", got)
	}
	if held, _ := site.server.instances.Get(id); held.Guidelines != "" {
		t.Error("a bio changed the room")
	}
}

func TestABioThatIsTooLongIsRefusedOnTheirPage(t *testing.T) {
	site, _, _, _ := aRecordedRoom(t, &roomWork{})
	got := site.as(t, bob).ask(t, "POST", "/members/people/"+url.PathEscape(bob)+"/bio", url.Values{"bio": {strings.Repeat("x", instance.MaxBio+1)}})
	if why := refusedTo(t, got.Header().Get("Location")); !strings.Contains(why, "at most") {
		t.Errorf("an over-long bio was answered with %q", why)
	}
	if site.server.instances.Bio(bob) != "" {
		t.Error("an over-long bio was kept")
	}
}

// A big room's bios stop at the brief's cap, whole people at a time.
func TestTheBriefHoldsOnlySoManyBios(t *testing.T) {
	site, id, _ := aRoom(t, &roomWork{})
	for i := range 40 {
		who := "person" + strings.Repeat("x", i) + "@example.com"
		if err := site.server.instances.SetMember(id, who, instance.RoleMember); err != nil {
			t.Fatal(err)
		}
		if err := site.server.instances.SetBio(who, strings.Repeat("word ", 100)); err != nil {
			t.Fatal(err)
		}
	}
	held, _ := site.server.instances.Get(id)
	got := site.server.peopleBios(held)
	if len(got) == 0 || len(got) > briefBios+100 {
		t.Errorf("the bios section is %d bytes against a cap of %d", len(got), briefBios)
	}
	if strings.Count(got, "\n- ") >= 40 {
		t.Error("no person was left out at the cap")
	}
	if !strings.HasSuffix(got, "\n") {
		t.Error("the cap cut a person's bio in two")
	}
}

func TestAPersonBioIsSaidInWordsInTheChangeRecord(t *testing.T) {
	if said := saidOf(changelog.Event{Kind: changelog.PersonBio, What: map[string]string{"now": "I run finance."}}); !strings.Contains(said, "I run finance.") {
		t.Errorf("a bio change is said as %q", said)
	}
	if said := saidOf(changelog.Event{Kind: changelog.PersonBio}); !strings.Contains(said, "cleared") {
		t.Errorf("clearing a bio is said as %q", said)
	}
}
