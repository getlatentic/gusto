package web

import (
	"strings"
	"testing"
	"time"
)

func TestAnAdminInvitesAPersonFromMembersByPostingToGustoCloud(t *testing.T) {
	admin := behindCloud(t)
	admin.cookie = cookieOf(t, admin.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	invite := admin.members(t, "/members").Invite
	if invite == nil || invite.Action != "https://cloud.test/org/invite?org=acme-com" || invite.Team != "main" {
		t.Errorf("an admin's Members page does not invite to gusto cloud's form: %+v", invite)
	}
}

func TestOnlyAnAdminOfAnOrganizationIsOfferedToInviteAPerson(t *testing.T) {
	runner := behindCloud(t)
	runner.cookie = cookieOf(t, runner.enterWith(t, tokenFor(t, "acme-com", "bob@acme.com", "runner", time.Now())))
	if runner.members(t, "/members").Invite != nil {
		t.Error("a runner is offered the invitation, which gusto cloud would refuse")
	}
	alone := open(t)
	if page := alone.as(t, alice).dataText(t, "/members"); !strings.Contains(page, `"Invite":null`) || strings.Contains(page, "/org/invite") {
		t.Error("a server with no gusto cloud has no organization to invite to, and its Members page offers one")
	}
}

func TestTheInvitationFormNamesThePageToComeBackTo(t *testing.T) {
	admin := behindCloud(t)
	admin.cookie = cookieOf(t, admin.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	if invite := admin.members(t, "/members").Invite; invite == nil || !strings.HasPrefix(invite.Back, "http://") || !strings.HasSuffix(invite.Back, "/members") {
		t.Error("the invitation form does not carry the address of the Members page for gusto cloud to send the person back to")
	}
}

func TestMembersSaysTheInvitationWasSentOnlyWhenGustoCloudSaysSo(t *testing.T) {
	admin := behindCloud(t)
	admin.cookie = cookieOf(t, admin.enterWith(t, tokenFor(t, "acme-com", "ada@acme.com", "admin", time.Now())))
	if !admin.members(t, "/members?sent=1").Sent {
		t.Error("Members does not say the invitation was sent after gusto cloud sends the person back with sent=1")
	}
	if admin.members(t, "/members").Sent {
		t.Error("Members says an invitation was sent when none was")
	}
	alone := open(t)
	if alone.as(t, alice).members(t, "/members?sent=1").Sent {
		t.Error("a server with no gusto cloud claims an invitation was sent")
	}
}

func TestMembersStillMakesAnAgentFromItsPlusWhereThereIsNoInvitation(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	if page := site.as(t, alice).members(t, "/members"); !page.Keeps || page.Invite != nil {
		t.Errorf("the + does not make an agent by itself where nobody can be invited: keeps %v, invite %+v", page.Keeps, page.Invite)
	}
}
