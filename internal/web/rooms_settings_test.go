package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/signing"
)

// aRecordedRoom is a room with bob in it that writes what changes to a change log.
func aRecordedRoom(t *testing.T, work *roomWork) (*site, *changelog.Log, string, string) {
	t.Helper()
	site, id, thread := aRoomWithBob(t, work)
	ring := t.TempDir()
	key, err := signing.Generate(ring, "acme")
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	log, err := changelog.Open(filepath.Join(t.TempDir(), "changes.jsonl"), key, keyring)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	site.server.UseKeyring(keyring)
	site.server.UseChanges(log)
	return site, log, id, thread
}

// railOf is the side list the person of this session is shown.
func railOf(t *testing.T, site *site, who string) railList {
	request := httptest.NewRequest("GET", "/chat", nil)
	request.AddCookie(site.as(t, who).cookie)
	return site.server.railFor(request, "")
}

func TestAnAdminWritesGuidelinesAndEveryAgentIsToldThemInEveryTurn(t *testing.T) {
	work := &roomWork{}
	site, log, id, thread := aRecordedRoom(t, work)
	mine, his := site.as(t, alice), site.as(t, bob)

	if got := his.ask(t, "POST", "/rooms/"+id+"/guidelines", url.Values{"guidelines": {"Answer in French."}}); got.Code != http.StatusForbidden {
		t.Fatalf("a member changing the guidelines answered %d", got.Code)
	}
	if got := mine.ask(t, "POST", "/rooms/"+id+"/guidelines", url.Values{"guidelines": {"Answer in French."}}); got.Code != http.StatusSeeOther || strings.Contains(got.Header().Get("Location"), "refused") {
		t.Fatalf("an admin changing the guidelines answered %d to %s", got.Code, got.Header().Get("Location"))
	}
	for _, said := range []string{"@Scout first", "@Scout second"} {
		his.ask(t, "POST", thread+"/say", url.Values{"said": {said}})
	}
	for i, turn := range work.waitTurns(t, site, 2) {
		if !strings.Contains(turn.Brief, "Standing instructions for this room") || !strings.Contains(turn.Brief, "Answer in French.") {
			t.Errorf("turn %d was not given the guidelines: %q", i, turn.Brief)
		}
	}
	if page := his.ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, "Answer in French.") || strings.Contains(page, `name="guidelines"`) {
		t.Error("a member sees the guidelines as text, and not as a form")
	}
	if page := mine.ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, `name="guidelines"`) {
		t.Error("an admin has no form to change the guidelines")
	}
	changed := ofKind(written(t, log), changelog.RoomGuidelines)
	if len(changed) != 1 || changed[0].Who != alice || changed[0].What["now"] != "Answer in French." {
		t.Errorf("the change record holds %+v", changed)
	}
}

func TestGuidelinesThatAreTooLongAreRefusedAndChangeNothing(t *testing.T) {
	site, log, id, _ := aRecordedRoom(t, &roomWork{})
	got := site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/guidelines", url.Values{"guidelines": {strings.Repeat("x", instance.MaxGuidelines+1)}})
	if why := refusedTo(t, got.Header().Get("Location")); !strings.Contains(why, "at most") || !strings.Contains(got.Header().Get("Location"), "guidelines=1") {
		t.Errorf("an over-long guideline was answered with %q at %s", why, got.Header().Get("Location"))
	}
	if held, _ := site.server.instances.Get(id); held.Guidelines != "" {
		t.Error("an over-long guideline was kept")
	}
	if len(ofKind(written(t, log), changelog.RoomGuidelines)) != 0 {
		t.Error("a refused change was recorded")
	}
	site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/guidelines", url.Values{"guidelines": {""}})
	if len(ofKind(written(t, log), changelog.RoomGuidelines)) != 0 {
		t.Error("clearing guidelines that were empty was recorded")
	}
}

func TestMutingARoomIsOnePersonsAndSilencesItsUnread(t *testing.T) {
	site, log, id, thread := aRecordedRoom(t, &roomWork{})
	site.as(t, alice).ask(t, "POST", thread+"/say", url.Values{"said": {"hello @Bob"}})
	if railOf(t, site, bob).Unread == 0 {
		t.Fatal("a message left nothing unread for bob before he muted the room")
	}

	if got := site.as(t, bob).ask(t, "POST", "/rooms/"+id+"/mute", url.Values{"mute": {"on"}}); got.Code != http.StatusSeeOther {
		t.Fatalf("muting answered %d", got.Code)
	}
	site.as(t, alice).ask(t, "POST", thread+"/say", url.Values{"said": {"again @Bob"}})
	if rail := railOf(t, site, bob); rail.Unread != 0 || rail.ForYou != 0 || rail.Rooms[0].Unread.Count != 0 {
		t.Errorf("a muted room still counts for bob: %+v", rail)
	}
	if railOf(t, site, alice).Rooms[0].Unread.Count != 0 || len(railOf(t, site, alice).Rooms) != 1 {
		t.Error("bob's mute changed what alice sees")
	}
	if held, _ := site.server.instances.Get(id); !held.MutedBy(bob) || held.MutedBy(alice) {
		t.Errorf("the mute is not bob's alone: %+v", held.Members)
	}
	if got := site.as(t, "eve@example.com").ask(t, "POST", "/rooms/"+id+"/mute", url.Values{"mute": {"on"}}); got.Code == http.StatusSeeOther {
		t.Error("somebody outside the room muted it")
	}

	site.as(t, bob).ask(t, "POST", "/rooms/"+id+"/mute", url.Values{"mute": {"off"}})
	if railOf(t, site, bob).Unread == 0 {
		t.Error("unmuting did not bring the unread back")
	}
	if got := ofKind(written(t, log), changelog.RoomMuted); len(got) != 2 || got[0].What["now"] != "on" || got[1].What["now"] != "off" {
		t.Errorf("the change record holds %+v", got)
	}
}

func TestAnArchivedRoomLeavesTheRailAndCannotBeSaidInUntilItIsBack(t *testing.T) {
	work := &roomWork{}
	site, log, id, thread := aRecordedRoom(t, work)
	mine, his := site.as(t, alice), site.as(t, bob)

	if got := his.ask(t, "POST", "/rooms/"+id+"/archive", url.Values{"archive": {"on"}}); got.Code != http.StatusForbidden {
		t.Fatalf("a member archiving answered %d", got.Code)
	}
	if got := mine.ask(t, "POST", "/rooms/"+id+"/archive", url.Values{"archive": {"on"}}); got.Code != http.StatusSeeOther {
		t.Fatalf("an admin archiving answered %d", got.Code)
	}
	if rail := railOf(t, site, bob); len(rail.Rooms) != 0 || len(rail.Archived) != 1 {
		t.Errorf("bob's rail holds %d rooms and %d archived", len(rail.Rooms), len(rail.Archived))
	}
	if page := mine.ask(t, "GET", "/chat", nil).Body.String(); !strings.Contains(page, "Archived rooms") {
		t.Error("the rail has no way to the archived rooms")
	}

	said := his.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout anyone there?"}})
	if why := refusedTo(t, said.Header().Get("Location")); !strings.Contains(why, "archived") {
		t.Errorf("saying something in an archived room was answered with %q", why)
	}
	if _, err := site.server.speak(bob, mustRoom(t, site, id), site.server.instances.Latest(id), "hello", "k1", false); err == nil {
		t.Error("the live path said something in an archived room")
	}
	if held := site.server.instances.Latest(id); len(held.Turns) != 0 || len(held.Notes) != 0 {
		t.Errorf("an archived room took in %d turns and %d notes", len(held.Turns), len(held.Notes))
	}
	if page := his.ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, "This room is archived") || strings.Contains(page, `value="off"`) {
		t.Error("a member sees the room as archived, with a way to bring it back")
	}
	if page := mine.ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, "Unarchive") {
		t.Error("an admin has no way to bring the room back")
	}

	mine.ask(t, "POST", "/rooms/"+id+"/archive", url.Values{"archive": {"off"}})
	if len(railOf(t, site, bob).Rooms) != 1 {
		t.Error("the room did not come back to the rail")
	}
	his.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout now?"}})
	work.waitTurns(t, site, 1)

	if got := ofKind(written(t, log), changelog.RoomArchived); len(got) != 2 || got[0].What["now"] != "on" || got[1].What["now"] != "off" {
		t.Errorf("the change record holds %+v", got)
	}
}

func mustRoom(t *testing.T, site *site, id string) instance.Instance {
	t.Helper()
	held, found := site.server.instances.Get(id)
	if !found {
		t.Fatal("the room is gone")
	}
	return held
}

func TestARoomsSettingsAreSaidInWordsInTheChangeRecord(t *testing.T) {
	for kind, want := range map[string]map[string]string{
		changelog.RoomGuidelines: {"room": "Plan", "now": "Be brief."},
		changelog.RoomArchived:   {"room": "Plan", "now": "on"},
		changelog.RoomMuted:      {"room": "Plan", "now": "off"},
	} {
		if said := saidOf(changelog.Event{Kind: kind, What: want}); !strings.Contains(said, "Plan") || said == kind {
			t.Errorf("%s is said as %q", kind, said)
		}
	}
}
