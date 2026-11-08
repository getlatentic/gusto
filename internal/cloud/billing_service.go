package cloud

import (
	"cmp"
	"context"
	"errors"
	"net/url"

	"github.com/getlatentic/gusto/internal/quota"
)

const settingPayPalPlan = "paypal_plan_id"

// Billing sells gusto cloud by the seat through PayPal. PayPal holds the payer and the card, and gusto holds
// each organization's subscription id, its seats and its state, which the organization's quota follows.
type Billing struct {
	Store  *Store
	PayPal *PayPalClient
	// Plan is what a paying organization may run, before its seats are put in. Zero is DefaultPlanQuota.
	Plan quota.Quota
	// PlanID is the PayPal plan to subscribe to. When it is empty the plan is made at PayPal once and remembered.
	PlanID string
	// URL is the front door's address, which PayPal sends a payer back to.
	URL *url.URL
}

// DefaultPlanQuota is what a paying organization may run: a generous month of tasks, since a seat is what is sold.
func DefaultPlanQuota() quota.Quota {
	return quota.Quota{Jobs: 2000, Monthly: true, Concurrent: 8, Invites: true}
}

func (b *Billing) plan() quota.Quota {
	if b.Plan == (quota.Quota{}) {
		return DefaultPlanQuota()
	}
	return b.Plan
}

func (b *Billing) planID(ctx context.Context) (string, error) {
	if b.PlanID != "" {
		return b.PlanID, nil
	}
	if id, found, err := b.Store.setting(settingPayPalPlan); err != nil || found {
		return id, err
	}
	id, err := b.PayPal.EnsurePlan(ctx)
	if err != nil {
		return "", err
	}
	return id, b.Store.saveSetting(settingPayPalPlan, id)
}

func (b *Billing) returnURL() string { return b.URL.JoinPath("org", "billing", "return").String() }
func (b *Billing) cancelURL() string { return b.URL.JoinPath("org").String() }

// Subscribe starts a subscription for an organization, with a seat for each of its people, and returns the
// address at PayPal the payer approves it at. While one still waits for its payer, asking again returns its address
// and makes no second subscription, since a payer may be approving the first at that moment.
func (b *Billing) Subscribe(ctx context.Context, org string) (string, error) {
	if waiting, err := b.waitingApproval(ctx, org); err != nil || waiting != "" {
		return waiting, err
	}
	members, err := b.Store.Members(org)
	if err != nil {
		return "", err
	}
	seats := max(len(members), 1)
	plan, err := b.planID(ctx)
	if err != nil {
		return "", err
	}
	nonce, err := randomID()
	if err != nil {
		return "", err
	}
	made, err := b.PayPal.CreateSubscription(ctx, plan, seats, org, b.returnURL(), b.cancelURL(), "gusto-"+org+"-"+nonce)
	if err != nil {
		return "", err
	}
	if made.ApproveURL == "" {
		return "", errors.New("paypal gave no address to approve the subscription at")
	}
	if err := b.Store.startSubscription(ctx, org, made.ID, made.ApproveURL, seats); err != nil {
		return "", err
	}
	return made.ApproveURL, nil
}

// waitingApproval is the address a pending subscription is approved at, read from PayPal because an address kept
// from earlier may have run out. It is empty when the organization has no subscription waiting. A subscription
// that PayPal says is no longer waiting is read in first, and is refused if it is paying.
func (b *Billing) waitingApproval(ctx context.Context, org string) (string, error) {
	record, found, err := b.Store.BillingOf(org)
	if err != nil || !found || record.State != BillingPending {
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
	if now, _, err := b.Store.BillingOf(org); err != nil || now.State != BillingSuspended {
		return "", errors.Join(err, ErrSubscribed)
	}
	return "", nil
}

// Return is where a payer lands after PayPal. It reads the organization's own subscription from PayPal and
// believes nothing in the address, so a payer who is back before the webhook sees their state at once.
func (b *Billing) Return(ctx context.Context, org string) error {
	record, found, err := b.Store.BillingOf(org)
	if err != nil || !found {
		return err
	}
	return b.Reconcile(ctx, record.SubscriptionID)
}

// Cancel ends an organization's subscription. Its people and work stay, and new tasks stop.
func (b *Billing) Cancel(ctx context.Context, org string) error {
	record, found, err := b.Store.BillingOf(org)
	if err != nil {
		return err
	}
	if !found || (record.State != BillingActive && record.State != BillingPastDue) {
		return ErrNotBilled
	}
	if err := b.PayPal.Cancel(ctx, record.SubscriptionID, "Cancelled by an admin in gusto"); err != nil {
		return err
	}
	return b.Reconcile(ctx, record.SubscriptionID)
}

// Approve is an admin's yes to a seat change. PayPal then needs the payer's own yes, at the address returned, and
// the seats change when it arrives. When PayPal asks for nothing more the change is applied at once.
func (b *Billing) Approve(ctx context.Context, org, proposalID, by string) (string, error) {
	record, found, err := b.Store.BillingOf(org)
	if err != nil {
		return "", err
	}
	if !found || (record.State != BillingActive && record.State != BillingPastDue) {
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
// say the same. A subscription that ends before anyone paid for it changes no quota. It is a function of what PayPal says now, so a repeat or an event out of order changes nothing.
func (b *Billing) Reconcile(ctx context.Context, subscriptionID string) error {
	owned, found, err := b.Store.billingBySubscription(subscriptionID)
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
	was, record, err := b.Store.applySubscription(ctx, owned.Org, answer, state)
	if err != nil || record.Org == "" || record.State == BillingPending || (was == BillingPending && record.State == BillingSuspended) {
		return err
	}
	return b.Store.setQuota(record.Org, paidQuota(b.plan(), record))
}

// stateOf is what a PayPal subscription means for the organization. A subscription still waiting for its payer
// has no state to apply yet.
func stateOf(s Subscription) (BillingState, bool) {
	switch s.Status {
	case "APPROVAL_PENDING", "APPROVED":
		return BillingPending, true
	case "ACTIVE":
		if s.FailedPayments > 0 {
			return BillingPastDue, true
		}
		return BillingActive, true
	case "SUSPENDED", "CANCELLED", "EXPIRED":
		return BillingSuspended, true
	}
	return "", false
}
