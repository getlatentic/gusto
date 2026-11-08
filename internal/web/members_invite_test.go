package web

import (
	"strings"
	"testing"
	"time"
)

func TestAnAdminInvitesAPersonFromMembersByPostingToGustoCloud(t *testing.T) {
	admin := behindCloud(t)
	admin.cookie = cookieOf(t, admin.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	page := admin.ask(t, "GET", "/members", nil).Body.String()
	for _, want := range []string{
		`action="https://cloud.test/org/invite?org=acme-com"`,
		`name="workspace" value="main"`,
		`name="email"`,
		"Invite a person",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("an admin's Members page lacks %s", want)
		}
	}
}

func TestOnlyAnAdminOfAnOrganizationIsOfferedToInviteAPerson(t *testing.T) {
	runner := behindCloud(t)
	runner.cookie = cookieOf(t, runner.enterWith(t, tokenFor(t, "acme-com", "bob@acme.com", "runner", time.Now())))
	if page := runner.ask(t, "GET", "/members", nil).Body.String(); strings.Contains(page, "invite-person") {
		t.Error("a runner is offered the invitation, which gusto cloud would refuse")
	}
	alone := open(t)
	if page := alone.as(t, alice).ask(t, "GET", "/members", nil).Body.String(); strings.Contains(page, "invite-person") || strings.Contains(page, "/org/invite") {
		t.Error("a server with no gusto cloud has no organization to invite to, and its Members page offers one")
	}
}

func TestTheInvitationFormNamesThePageToComeBackTo(t *testing.T) {
	admin := behindCloud(t)
	admin.cookie = cookieOf(t, admin.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	page := admin.ask(t, "GET", "/members", nil).Body.String()
	if !strings.Contains(page, `name="back" value="http://`) || !strings.Contains(page, `/members"`) {
		t.Error("the invitation form does not carry the address of the Members page for gusto cloud to send the person back to")
	}
}

func TestMembersSaysTheInvitationWasSentOnlyWhenGustoCloudSaysSo(t *testing.T) {
	admin := behindCloud(t)
	admin.cookie = cookieOf(t, admin.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	if page := admin.ask(t, "GET", "/members?sent=1", nil).Body.String(); !strings.Contains(page, "The invitation was sent") {
		t.Error("Members does not say the invitation was sent after gusto cloud sends the person back with sent=1")
	}
	if page := admin.ask(t, "GET", "/members", nil).Body.String(); strings.Contains(page, "The invitation was sent") {
		t.Error("Members says an invitation was sent when none was")
	}
	alone := open(t)
	if page := alone.as(t, alice).ask(t, "GET", "/members?sent=1", nil).Body.String(); strings.Contains(page, "The invitation was sent") {
		t.Error("a server with no gusto cloud claims an invitation was sent")
	}
}

func TestMembersStillMakesAnAgentFromItsPlusWhereThereIsNoInvitation(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	page := site.as(t, alice).ask(t, "GET", "/members", nil).Body.String()
	if !strings.Contains(page, `data-open="new-agent"`) || strings.Contains(page, "Add a member") {
		t.Error("the + opens the new agent dialog by itself where nobody can be invited")
	}
}
