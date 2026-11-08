package cloud

import (
	"cmp"
	"context"
	"errors"
	"net/url"
)

const settingPayPalPlan = "paypal_plan_id"

// PayPalBilling sells gusto cloud by the seat through PayPal. PayPal holds the payer and the card, and gusto holds
// each organization's subscription id, its seats and its state, which the organization's quota follows.
type PayPalBilling struct {
	Store  *Store
	PayPal *PayPalClient
	// PlanID is the PayPal plan to subscribe to. When it is empty the plan is made at PayPal once and remembered.
	PlanID string
	// URL is the front door's address, which PayPal sends a payer back to.
	URL *url.URL
}

func (b *PayPalBilling) planID(ctx context.Context) (string, error) {
	if b.PlanID != "" {
		return b.PlanID, nil
	}
	if id, found, err := b.Store.setting(ctx, settingPayPalPlan); err != nil || found {
		return id, err
	}
	id, err := b.PayPal.EnsurePlan(ctx)
	if err != nil {
		return "", err
	}
	return id, b.Store.saveSetting(ctx, settingPayPalPlan, id)
}

// useURL gives the front door's address to billing that was made without one. It does nothing for no billing.
func (b *PayPalBilling) useURL(u *url.URL) {
	if b != nil && b.URL == nil {
		b.URL = u
	}
}

func (b *PayPalBilling) returnURL() string {
	return b.URL.JoinPath("org", "billing", "return").String()
}
func (b *PayPalBilling) cancelURL() string { return b.URL.JoinPath("org").String() }

// Subscribe starts a subscription for an organization, with a seat for each of its people, and returns the
// address at PayPal the payer approves it at. While one still waits for its payer, asking again returns its address
// and makes no second subscription, since a payer may be approving the first at that moment. An organization that
// pays through Stripe is refused: one organization is billed one way.
func (b *PayPalBilling) Subscribe(ctx context.Context, org string) (string, error) {
	if err := b.refuseIfBilledByStripe(ctx, org); err != nil {
		return "", err
	}
	if waiting, err := b.waitingApproval(ctx, org); err != nil || waiting != "" {
		return waiting, err
	}
	members, err := b.Store.Members(ctx, org)
	if err != nil {
		return "", err
	}
	seats := max(len(members), 1)
	made, err := b.create(ctx, org, seats)
	if err != nil {
		return "", err
	}
	if err := b.Store.startSubscription(ctx, org, made.ID, made.ApproveURL, seats); err != nil {
		return "", err
	}
	return made.ApproveURL, nil
}

func (b *PayPalBilling) refuseIfBilledByStripe(ctx context.Context, org string) error {
	stripe, found, err := b.Store.Billing(ctx, org)
	if err == nil && found && (stripe.Paid() || stripe.Lapsed()) {
		err = ErrSubscribed
	}
	return err
}

// create makes the subscription at PayPal. Its request id is new each time, so a repeat of the call is a new subscription.
func (b *PayPalBilling) create(ctx context.Context, org string, seats int) (Subscription, error) {
	plan, err := b.planID(ctx)
	if err != nil {
		return Subscription{}, err
	}
	nonce, err := randomID()
	if err != nil {
		return Subscription{}, err
	}
	made, err := b.PayPal.CreateSubscription(ctx, plan, seats, org, b.returnURL(), b.cancelURL(), "gusto-"+org+"-"+nonce)
	if err == nil && made.ApproveURL == "" {
		err = errors.New("paypal gave no address to approve the subscription at")
	}
	return made, err
}

// waitingApproval is the address a pending subscription is approved at, read from PayPal because an address kept
// from earlier may have run out. It is empty when the organization has no subscription waiting. A subscription
// that PayPal says is no longer waiting is read in first, and is refused if it is paying.
func (b *PayPalBilling) waitingApproval(ctx context.Context, org string) (string, error) {
	record, found, err := b.Store.PayPalOf(org)
	if err != nil || !found || record.State != PayPalPending {
		return "", err
	}
	answer, err := b.PayPal.Subscription(ctx, record.SubscriptionID)
	if err != nil {
		return "", err
	}
	if answer.Status == "APPROVAL_PENDING" {
		return cmp.Or(answer.ApproveURL, record.ApproveURL), nil
	}
	if err := b.Reconcile(ctx, record.SubscriptionID); err != nil {
		return "", err
	}
	if now, _, err := b.Store.PayPalOf(org); err != nil || now.State != PayPalSuspended {
		return "", errors.Join(err, ErrSubscribed)
	}
	return "", nil
}

// Return is where a payer lands after PayPal. It reads the organization's own subscription from PayPal and
// believes nothing in the address, so a payer who is back before the webhook sees their state at once.
func (b *PayPalBilling) Return(ctx context.Context, org string) error {
	record, found, err := b.Store.PayPalOf(org)
	if err != nil || !found {
		return err
	}
	return b.Reconcile(ctx, record.SubscriptionID)
}

// Cancel ends an organization's subscription. Its people and work stay, and new tasks stop.
func (b *PayPalBilling) Cancel(ctx context.Context, org string) error {
	record, found, err := b.Store.PayPalOf(org)
	if err != nil {
		return err
	}
	if !found || (record.State != PayPalActive && record.State != PayPalPastDue) {
		return ErrNotBilled
	}
	if err := b.PayPal.Cancel(ctx, record.SubscriptionID, "Cancelled by an admin in gusto"); err != nil {
		return err
	}
	return b.Reconcile(ctx, record.SubscriptionID)
}

// Approve is an admin's yes to a seat change. PayPal then needs the payer's own yes, at the address returned, and
// the seats change when it arrives. When PayPal asks for nothing more the change is applied at once.
func (b *PayPalBilling) Approve(ctx context.Context, org, proposalID, by string) (string, error) {
	record, found, err := b.Store.PayPalOf(org)
	if err != nil {
		return "", err
	}
	if !found || (record.State != PayPalActive && record.State != PayPalPastDue) {
		return "", ErrNotBilled
	}
	proposal, err := b.Store.claimProposal(ctx, org, proposalID, by)
	if err != nil {
		return "", err
	}
	approveURL, err := b.PayPal.ReviseSeats(ctx, record.SubscriptionID, proposal.Seats, b.returnURL(), b.cancelURL())
	if err != nil {
		return "", errors.Join(err, b.Store.releaseProposal(ctx, proposal.ID))
	}
	if err := b.Store.keepApproval(ctx, proposal.ID, approveURL); err != nil {
		return "", err
	}
	if approveURL == "" {
		return "", b.Reconcile(ctx, record.SubscriptionID)
	}
	return approveURL, nil
}

// Reconcile reads a subscription's current state from PayPal and makes the organization's record and quota
// say the same. It is a function of what PayPal says now, so a repeat or an event out of order changes nothing.
func (b *PayPalBilling) Reconcile(ctx context.Context, subscriptionID string) error {
	owned, found, err := b.Store.payPalBySubscription(subscriptionID)
	if err != nil || !found {
		return err
	}
	answer, err := b.PayPal.Subscription(ctx, subscriptionID)
	if err != nil {
		return err
	}
	state, known := stateOf(answer)
	if !known {
		return nil
	}
	if _, err := b.Store.applySubscription(ctx, owned.Org, answer, state); err != nil {
		return err
	}
	b.Store.quotaChanged(ctx, owned.Org)
	return nil
}

// stateOf is what a PayPal subscription means for the organization. A subscription still waiting for its payer
// has no state to apply yet.
func stateOf(s Subscription) (PayPalState, bool) {
	switch s.Status {
	case "APPROVAL_PENDING", "APPROVED":
		return PayPalPending, true
	case "ACTIVE":
		if s.FailedPayments > 0 {
			return PayPalPastDue, true
		}
		return PayPalActive, true
	case "SUSPENDED", "CANCELLED", "EXPIRED":
		return PayPalSuspended, true
	}
	return "", false
}
