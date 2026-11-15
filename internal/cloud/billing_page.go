package cloud

import "github.com/getlatentic/gusto/internal/cloud/payments"

// billingPage is the organization's billing as its page draws it: the plan in words, whether it is paid up, the seats, and what an
// admin may do about it.
type billingPage struct {
	Admin           bool
	Active          bool
	Words, Provider string
	Seats, People   int
	Offers          []billingOffer
	Manage, Cancel  bool
	Proposals       []proposalPage
}

// proposalPage is a proposed change of seats: who proposed it, why, and where it is.
type proposalPage struct {
	ID, Reason, ApproveURL string
	Seats                  int
	ByAgent                bool
	// Pending is waiting for an admin, and Awaiting for the payer to approve it at the processor.
	Pending, Awaiting bool
}

func billingPageOf(v *billingView) *billingPage {
	if v == nil {
		return nil
	}
	page := &billingPage{
		Admin: v.Admin, Active: v.State == payments.Active, Words: v.Words, Provider: v.Provider,
		Seats: v.Seats, People: v.People, Offers: v.Offers, Manage: v.Manage, Cancel: v.Cancel,
	}
	for _, p := range v.Proposals {
		page.Proposals = append(page.Proposals, proposalPage{
			ID: p.ID, Reason: p.Reason, ApproveURL: p.ApproveURL, Seats: p.Seats, ByAgent: p.Source == "agent",
			Pending: p.Status == ProposalPending, Awaiting: p.Status == ProposalAwaiting,
		})
	}
	return page
}
