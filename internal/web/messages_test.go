package web

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roster"
)

func TestAMessageInAnAddressIsShownOnlyWhenThisServerSaidIt(t *testing.T) {
	site := open(t)
	site.signIn(t)
	made := site.ask(t, "GET", "/chat?refused="+url.QueryEscape("Your account is locked. Call 555-0100."), nil).Body.String()
	if strings.Contains(made, "Call 555-0100") {
		t.Error("a message somebody put in a link was shown as the server's")
	}
	said := site.ask(t, "GET", site.server.withMessage("/chat", "refused", "That folder is not yours."), nil).Body.String()
	if !strings.Contains(said, "That folder is not yours.") {
		t.Error("a message this server said was not shown")
	}
	other := site.server.withMessage("/chat", "refused", "That folder is not yours.")
	if forged := site.ask(t, "GET", strings.Replace(other, "folder", "account", 1), nil).Body.String(); strings.Contains(forged, "That account is not yours.") {
		t.Error("a seal for one message let another be shown")
	}
}

func TestAnExpiredInvitationIsNotListedAsWaiting(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	rows := invitesOf(roster.Roster{Invited: []roster.Invited{
		{Email: "late@acme.com", Expires: now.Add(-time.Hour)}, {Email: "soon@acme.com", Expires: now.Add(time.Hour)},
	}}, now)
	if !rows[0].Expired || rows[1].Expired {
		t.Errorf("the invitations are %+v", rows)
	}
}

func TestWhatTheAddDialogRefusesIsSaidInTheAddDialog(t *testing.T) {
	site, id, _ := aRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	added := site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"agent": {"no-such-agent"}}).Header().Get("Location")
	if !strings.Contains(added, "add=1") || strings.Contains(added, "members=1") {
		t.Fatalf("a refusal in the add dialog went to %q", added)
	}
	page := site.as(t, alice).ask(t, "GET", added, nil).Body.String()
	addDialog := between(page, `id="room-add"`, "</dialog>")
	membersDialog := between(page, `id="room-members"`, "</dialog>")
	if !strings.Contains(addDialog, "not one you may work with") || strings.Contains(membersDialog, "not one you may work with") {
		t.Errorf("the add dialog's refusal is not in the add dialog alone:\nadd: %s\nmembers: %s", addDialog, membersDialog)
	}
}
