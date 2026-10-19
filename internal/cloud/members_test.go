package cloud

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/quota"
)

// verifiedOrg is an organisation whose admin has confirmed their address, which is
// what lets it invite, with the admin as a person.
func verifiedOrg(t *testing.T, s *Store, email, name string) (Person, Org) {
	t.Helper()
	person, org, err := s.SignUp(Signup{Email: email, OrgName: name})
	if err != nil {
		t.Fatal(err)
	}
	if person, err = s.Verify(email); err != nil {
		t.Fatal(err)
	}
	return person, org
}

func joined(t *testing.T, s *Store, org Org, admin Person, email string) Person {
	t.Helper()
	secret, err := s.Invite(org.Slug, admin, email)
	if err != nil {
		t.Fatalf("inviting %s: %v", email, err)
	}
	person, _, err := s.Accept(secret)
	if err != nil {
		t.Fatalf("accepting for %s: %v", email, err)
	}
	return person
}

func TestOnlyAnAdminOfAnOrganisationMayChangeARoleInIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		ada, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		bob := joined(t, s, org, ada, "bob@acme.com")
		eve, _ := verifiedOrg(t, s, "eve@globex.com", "Globex")
		if _, err := s.SetRole(org.Slug, bob, "bob@acme.com", Admin); !errors.Is(err, ErrNotAdmin) {
			t.Errorf("a runner cannot: %v", err)
		}
		if _, err := s.SetRole(org.Slug, eve, "bob@acme.com", Admin); !errors.Is(err, ErrNotAdmin) {
			t.Errorf("an admin of another organisation cannot: %v", err)
		}
		if was, err := s.SetRole(org.Slug, ada, "BOB@acme.com", Admin); err != nil || was != Runner {
			t.Errorf("an admin can, at an address written in any case: %v %v", was, err)
		}
		if got, _, _ := s.Person("bob@acme.com"); got.Role != Admin {
			t.Errorf("%+v", got)
		}
	})
}

func TestOnlyAnAdminOfAnOrganisationWhoseAddressIsConfirmedMayInvite(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		ada, org, _ := s.SignUp(Signup{Email: "ada@acme.com", OrgName: "Acme"})
		if _, err := s.Invite(org.Slug, ada, "bob@acme.com"); !errors.Is(err, ErrNoInvites) {
			t.Fatalf("an organisation on the trial cannot invite: %v", err)
		}
		admin, _ := s.Verify("ada@acme.com")
		secret, err := s.Invite(org.Slug, admin, "Bob@Acme.com")
		if err != nil || secret == "" {
			t.Fatalf("%v", err)
		}
		bob := joined(t, s, org, admin, "cy@acme.com")
		if _, err := s.Invite(org.Slug, bob, "di@acme.com"); !errors.Is(err, ErrNotAdmin) {
			t.Errorf("a runner cannot invite: %v", err)
		}
		other, otherOrg := verifiedOrg(t, s, "eve@globex.com", "Globex")
		if _, err := s.Invite(org.Slug, other, "fay@globex.com"); !errors.Is(err, ErrNotAdmin) {
			t.Errorf("an admin of another organisation cannot invite into this one: %v", err)
		}
		if _, err := s.Invite(otherOrg.Slug, admin, "fay@globex.com"); !errors.Is(err, ErrNotAdmin) {
			t.Errorf("nor can this one's into that: %v", err)
		}
		for _, bad := range []string{"not an address", "x@mailinator.com"} {
			if _, err := s.Invite(org.Slug, admin, bad); err == nil {
				t.Errorf("%q should not be invited", bad)
			}
		}
		if _, err := s.Invite(org.Slug, admin, "gus@gmail.com"); err != nil {
			t.Errorf("a contractor at a free mail address may be invited: %v", err)
		}
	})
}

func TestAnInvitedPersonJoinsAsAVerifiedRunnerAndTheirInvitationIsSpent(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		secret, _ := s.Invite(org.Slug, admin, "bob@acme.com")
		if open, _ := s.Invitations(org.Slug); len(open) != 1 || open[0].Email != "bob@acme.com" || open[0].By != "ada@acme.com" {
			t.Fatalf("%+v", open)
		}
		person, joinedOrg, err := s.Accept(secret)
		if err != nil {
			t.Fatal(err)
		}
		if person.Email != "bob@acme.com" || person.Role != Runner || person.Org != org.Slug || person.Trial() || person.By != "ada@acme.com" || joinedOrg.Slug != org.Slug {
			t.Errorf("%+v", person)
		}
		if open, _ := s.Invitations(org.Slug); len(open) != 0 {
			t.Errorf("an answered invitation is not open: %+v", open)
		}
		if _, _, err := s.Accept(secret); !errors.Is(err, ErrLink) {
			t.Errorf("an invitation is answered once: %v", err)
		}
		members, _ := s.Members(org.Slug)
		if len(members) != 2 || members[0].Email != "ada@acme.com" || members[1].Email != "bob@acme.com" {
			t.Errorf("admins first: %+v", members)
		}
		// Their being verified changes nothing for the organisation: only its admin's address does that.
		if q, _ := s.Quota(org.Slug); q != DefaultLimits().Verified {
			t.Errorf("%+v", q)
		}
	})
}

func TestAnAddressThatHasAnAccountCannotBeInvitedAndTheAnswerIsTheSameWhoseItIs(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		verifiedOrg(t, s, "eve@globex.com", "Globex")
		s.SignUp(Signup{Email: "ned@initech.com", OrgName: "Initech"})
		for _, taken := range []string{"eve@globex.com", "ned@initech.com", "ada@acme.com"} {
			if _, err := s.Invite(org.Slug, admin, taken); !errors.Is(err, ErrCannotInvite) {
				t.Errorf("%s: %v", taken, err)
			}
		}
		// Somebody who signed up between the invitation and its answer is refused at the answer.
		secret, _ := s.Invite(org.Slug, admin, "late@acme.com")
		s.SignUp(Signup{Email: "late@acme.com", OrgName: "Late Ltd", Slug: "late-ltd"})
		if _, _, err := s.Accept(secret); !errors.Is(err, ErrEmailTaken) {
			t.Errorf("%v", err)
		}
	})
}

func TestAnOrganisationHasNoMorePeopleThanItMayCountingThoseInvited(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme") // an admin and three more
		s.Invite(org.Slug, admin, "a@acme.com")
		s.Invite(org.Slug, admin, "b@acme.com")
		s.Invite(org.Slug, admin, "c@acme.com")
		if _, err := s.Invite(org.Slug, admin, "d@acme.com"); !errors.Is(err, ErrFull) {
			t.Fatalf("invitations count: %v", err)
		}
		// Inviting an address again does not count it twice, and cancelling gives the place back.
		if _, err := s.Invite(org.Slug, admin, "a@acme.com"); err != nil {
			t.Errorf("the same address again: %v", err)
		}
		if err := s.Cancel(org.Slug, admin, "c@acme.com"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Invite(org.Slug, admin, "d@acme.com"); err != nil {
			t.Errorf("a cancelled invitation freed its place: %v", err)
		}
	})
}

// The room is checked when the invitation is answered too: the organisation may have been
// given fewer places while it was out.
func TestAnInvitationIsRefusedWhenTheOrganisationHasFilledSinceItWasSent(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		secret, _ := s.Invite(org.Slug, admin, "bob@acme.com")
		if err := quota.Write(s.QuotaFile(org.Slug), quota.Quota{Jobs: 10, Members: 1, Computers: 1, Invites: true}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Accept(secret); !errors.Is(err, ErrFull) {
			t.Errorf("%v", err)
		}
		if _, found, _ := s.Person("bob@acme.com"); found {
			t.Error("a person was made of a refused invitation")
		}
	})
}

func TestACancelledInvitationsLinkNoLongerWorksAndInvitingAgainReplacesTheEarlierLink(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		first, _ := s.Invite(org.Slug, admin, "bob@acme.com")
		second, _ := s.Invite(org.Slug, admin, "bob@acme.com")
		if _, _, err := s.Accept(first); !errors.Is(err, ErrLink) {
			t.Errorf("the earlier link is taken away: %v", err)
		}
		if open, _ := s.Invitations(org.Slug); len(open) != 1 {
			t.Fatalf("one invitation is open: %+v", open)
		}
		if err := s.Cancel(org.Slug, admin, "bob@acme.com"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Accept(second); !errors.Is(err, ErrLink) {
			t.Errorf("a cancelled invitation's link does not work: %v", err)
		}
		if err := s.Cancel(org.Slug, admin, "nobody@acme.com"); err != nil {
			t.Errorf("cancelling what is not there is not an error: %v", err)
		}
		runner := joined(t, s, org, admin, "cy@acme.com")
		if err := s.Cancel(org.Slug, runner, "bob@acme.com"); !errors.Is(err, ErrNotAdmin) {
			t.Errorf("%v", err)
		}
	})
}

func TestAnInvitationRunsOutAfterAWeekAndIsNotListedAfterIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
		s.now = func() time.Time { return now }
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		secret, _ := s.Invite(org.Slug, admin, "bob@acme.com")
		now = now.Add(InviteLife - time.Hour)
		if open, _ := s.Invitations(org.Slug); len(open) != 1 {
			t.Errorf("still open: %+v", open)
		}
		now = now.Add(2 * time.Hour)
		if open, _ := s.Invitations(org.Slug); len(open) != 0 {
			t.Errorf("run out: %+v", open)
		}
		if _, _, err := s.Accept(secret); !errors.Is(err, ErrLink) {
			t.Errorf("%v", err)
		}
		if _, err := s.Invite(org.Slug, admin, "bob@acme.com"); err != nil {
			t.Errorf("an address whose invitation ran out can be invited again: %v", err)
		}
	})
}

func TestALinkIsForItsOwnPurposeAndAnInvitationIsNotASignIn(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		invite, _ := s.Invite(org.Slug, admin, "bob@acme.com")
		if _, err := s.UseLink(invite, SignIn); !errors.Is(err, ErrLink) {
			t.Errorf("an invitation does not sign anyone in: %v", err)
		}
		invite, _ = s.Invite(org.Slug, admin, "bob@acme.com")
		verify, _ := s.MakeLink("ada@acme.com", Verify, time.Hour)
		if _, _, err := s.Accept(verify); !errors.Is(err, ErrLink) {
			t.Errorf("a link to verify is not an invitation: %v", err)
		}
		if _, _, err := s.Accept(invite); err != nil {
			t.Errorf("%v", err)
		}
	})
}

func TestAnAdminRemovesPeopleButNotThemselvesOrTheLastAdmin(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		bob := joined(t, s, org, admin, "bob@acme.com")
		if err := s.Remove(org.Slug, bob, "ada@acme.com"); !errors.Is(err, ErrNotAdmin) {
			t.Errorf("a runner removes nobody: %v", err)
		}
		if err := s.Remove(org.Slug, admin, "ada@acme.com"); !errors.Is(err, ErrLastAdmin) {
			t.Errorf("the last admin stays: %v", err)
		}
		other, _ := verifiedOrg(t, s, "eve@globex.com", "Globex")
		if err := s.Remove(org.Slug, other, "bob@acme.com"); !errors.Is(err, ErrNotAdmin) {
			t.Errorf("another organisation's admin removes nobody here: %v", err)
		}
		if err := s.Remove(org.Slug, admin, "eve@globex.com"); !errors.Is(err, ErrNoMember) {
			t.Errorf("a person of another organisation is not here to remove: %v", err)
		}
		if err := s.Remove(org.Slug, admin, "nobody@acme.com"); !errors.Is(err, ErrNoMember) {
			t.Errorf("%v", err)
		}
		if err := s.Remove(org.Slug, admin, "Bob@Acme.com"); err != nil {
			t.Fatal(err)
		}
		if _, found, _ := s.Person("bob@acme.com"); found {
			t.Error("a removed person still has an account")
		}
		if members, _ := s.Members(org.Slug); len(members) != 1 {
			t.Errorf("%+v", members)
		}
		// A second admin can be removed by the first, and cannot remove the first once they are alone.
		secret, _ := s.Invite(org.Slug, admin, "cy@acme.com")
		cy, _, _ := s.Accept(secret)
		if _, err := s.db.ExecContext(context.Background(), `UPDATE people SET role = ? WHERE email = ?`, string(Admin), cy.Email); err != nil {
			t.Fatal(err)
		}
		cy.Role = Admin
		if err := s.Remove(org.Slug, cy, "ada@acme.com"); err != nil {
			t.Errorf("with two admins one may remove the other: %v", err)
		}
		if err := s.Remove(org.Slug, cy, "cy@acme.com"); !errors.Is(err, ErrLastAdmin) {
			t.Errorf("%v", err)
		}
	})
}

func TestInvitationsMadeTogetherNeverPassTheNumberOfPeopleAllowed(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		var wg sync.WaitGroup
		var mu sync.Mutex
		sent := 0
		for i := range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := s.Invite(org.Slug, admin, fmt.Sprintf("p%d@acme.com", i)); err == nil {
					mu.Lock()
					sent++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if open, _ := s.Invitations(org.Slug); sent != 3 || len(open) != 3 {
			t.Errorf("%d were sent and %d are open, and an admin and three is all that is allowed", sent, len(open))
		}
	})
}

func TestOneInvitationMakesOnePersonHoweverManyTimeItIsFollowed(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		secret, _ := s.Invite(org.Slug, admin, "bob@acme.com")
		var wg sync.WaitGroup
		var mu sync.Mutex
		joinedCount := 0
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, _, err := s.Accept(secret); err == nil {
					mu.Lock()
					joinedCount++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if members, _ := s.Members(org.Slug); joinedCount != 1 || len(members) != 2 {
			t.Errorf("%d joined and %d are in", joinedCount, len(members))
		}
	})
}

// A person's computer is marked for removal when they are removed, by the front door, which
// leaves the removing to the organisation's server.
func TestRemovingAPersonMarksTheirComputerForRemoval(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		admin, org := verifiedOrg(t, s, "ada@acme.com", "Acme")
		bob := joined(t, s, org, admin, "bob@acme.com")
		cy := joined(t, s, org, admin, "cy@acme.com")
		registry, err := s.Computers(org.Slug)
		if err != nil {
			t.Fatal(err)
		}
		registry.Ensure(bob.Email, "local")
		registry.Ensure(cy.Email, "local")
		if err := s.Remove(org.Slug, admin, "bob@acme.com"); err != nil {
			t.Fatal(err)
		}
		retiring, _ := registry.Retiring()
		if len(retiring) != 1 || retiring[0].User != "bob@acme.com" {
			t.Errorf("only the removed person's computer is marked: %+v", retiring)
		}
		// A person who never had one is removed with nothing to mark.
		dee := joined(t, s, org, admin, "dee@acme.com")
		if err := s.Remove(org.Slug, admin, dee.Email); err != nil {
			t.Errorf("%v", err)
		}
		if retiring, _ := registry.Retiring(); len(retiring) != 1 {
			t.Errorf("%+v", retiring)
		}
	})
}
