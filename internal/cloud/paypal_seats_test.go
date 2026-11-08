package cloud

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestSeatsAreOnlyProposedByAnOrganizationThatPaysForANumberItCanHave(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		s := b.front.store
		if _, err := s.Propose(b.org, 3, "growing", "agent"); !errors.Is(err, ErrNotBilled) {
			t.Errorf("an organization that does not pay cannot: %v", err)
		}
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		for _, seats := range []int{0, 1, MostSeats + 1} {
			if _, err := s.Propose(b.org, seats, "x", "agent"); !errors.Is(err, ErrSeats) {
				t.Errorf("%d seats: %v", seats, err)
			}
		}
		ada, _, _ := s.Person(t.Context(), "ada@acme.com", b.org)
		if _, err := s.Invite(t.Context(), b.org, ada, "bob@acme.com"); !errors.Is(err, ErrFull) {
			t.Fatalf("one paid seat has room for one person: %v", err)
		}
		if _, err := s.Propose(b.org, 3, "two colleagues are joining", "agent"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Propose(b.org, 4, "more", "agent"); !errors.Is(err, ErrProposalOpen) {
			t.Errorf("a second open proposal: %v", err)
		}
	})
}

func TestASeatChangeNeedsAnAdminAndThenThePayerBeforeAnythingChanges(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		s := b.front.store
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		proposal, err := s.Propose(b.org, 3, "two colleagues are joining", "agent")
		if err != nil {
			t.Fatal(err)
		}
		if code, _, header := b.admin.post("/org/paypal/proposals/approve", url.Values{"id": {proposal.ID}}); code != 303 || !strings.Contains(header.Get("Location"), "sandbox.paypal.com") {
			t.Fatalf("approving answered %d to %q", code, header.Get("Location"))
		}
		if b.record().Seats != 1 || b.quota().Members != 1 {
			t.Errorf("seats changed before the payer approved: %+v", b.record())
		}
		if code, _, _ := b.admin.post("/org/paypal/proposals/approve", url.Values{"id": {proposal.ID}}); code != 409 {
			t.Errorf("approving the same proposal twice answered %d", code)
		}
		b.fake.payerApproves(id)
		b.deliver("WH-2", "BILLING.SUBSCRIPTION.UPDATED", id, 204)
		open, _ := s.OpenProposals(b.org)
		if b.record().Seats != 3 || b.quota().Members != 3 || b.quota().Computers != 3 || len(open) != 0 {
			t.Errorf("%+v %+v %v", b.record(), b.quota(), open)
		}
	})
}

func TestASeatChangePayPalRefusesGoesBackToWaitingForAnAdmin(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		proposal, _ := b.front.store.Propose(b.org, 3, "growing", "agent")
		b.fake.reviseFails = true
		if code, _, _ := b.admin.post("/org/paypal/proposals/approve", url.Values{"id": {proposal.ID}}); code != 502 {
			t.Fatalf("a refusal from PayPal answered %d", code)
		}
		open, _ := b.front.store.OpenProposals(b.org)
		if len(open) != 1 || open[0].Status != ProposalPending {
			t.Errorf("%+v", open)
		}
	})
}

func TestADeclinedProposalFreesTheOrganizationToAskAgain(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		b := newBilled(t)
		_, id := b.subscribe()
		b.activate(id, "WH-1")
		proposal, _ := b.front.store.Propose(b.org, 3, "growing", "agent")
		if code, _, _ := b.admin.post("/org/paypal/proposals/decline", url.Values{"id": {proposal.ID}}); code != 303 {
			t.Fatalf("declining answered %d", code)
		}
		if _, err := b.front.store.Propose(b.org, 4, "again", "agent"); err != nil {
			t.Errorf("asking again: %v", err)
		}
	})
}
