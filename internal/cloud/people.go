package cloud

import (
	"time"

	"github.com/getlatentic/gusto/internal/quota"
)

// Role is what a person may do in their organization.
type Role string

const (
	// Admin makes the organization's policy and invites people.
	Admin Role = "admin"
	// Runner runs agents under the policy.
	Runner Role = "runner"
)

// Person is one address, in one organization. Verified is when they showed they
// read their mail, and is the zero time until then.
type Person struct {
	Email    string    `json:"email"`
	Org      string    `json:"org"`
	Role     Role      `json:"role"`
	Created  time.Time `json:"created"`
	Verified time.Time `json:"verified,omitzero"`
	// By is who invited them, and empty for the admin who made the organization.
	By string `json:"by,omitempty"`
	// LastSignIn is when the front door last signed them in to their organization's
	// server, and the zero time until it has.
	LastSignIn time.Time `json:"lastSignIn,omitzero"`
}

// Org is an organization as the front door knows it. Its work is in its own
// directory and its own server, and this is only who it is and who made it.
type Org struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Admin string `json:"admin"`
	// Domain is the company's domain when the admin signed up from one, and is
	// empty for an address at a free mail provider.
	Domain  string    `json:"domain,omitempty"`
	Created time.Time `json:"created"`
	// Demo is set when the organization is a demo, which is deleted when its time is up.
	Demo *Demo `json:"demo,omitempty"`
}

// Limits are the quotas an organization moves between, and the settings that make
// them. Paid is for an organization with a subscription, and its Jobs are for each seat.
type Limits struct {
	Trial, Verified, Paid quota.Quota
}

// StartingCredit is what a new organization may spend on the models gusto provides, once. After it, an organization adds its own
// model key.
const StartingCredit = 5.0

// DefaultLimits are what a sign-up may run before and after it is verified, and once it pays.
func DefaultLimits() Limits {
	return Limits{
		Trial:    quota.Quota{Jobs: 3, Concurrent: 1, Members: 1, Computers: 1, Note: "Confirm the admin's email address to raise this limit.", CreditUSD: StartingCredit},
		Verified: quota.Quota{Jobs: 200, Monthly: true, Concurrent: 4, Invites: true, Members: 4, Computers: 4, CreditUSD: StartingCredit},
		Paid:     quota.Quota{Jobs: 200, Monthly: true, Concurrent: 4, Invites: true, Members: 50, Computers: 50, CreditUSD: StartingCredit},
	}
}

// Trial says a person's organization is still on the small quota.
func (p Person) Trial() bool { return p.Verified.IsZero() }
