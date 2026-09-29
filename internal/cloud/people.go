package cloud

import (
	"time"

	"github.com/getlatentic/haas/internal/quota"
)

// Role is what a person may do in their organisation.
type Role string

const (
	// Admin makes the organisation's policy and invites people.
	Admin Role = "admin"
	// Runner runs agents under the policy.
	Runner Role = "runner"
)

// Person is one address, in one organisation. Verified is when they showed they
// read their mail, and is the zero time until then.
type Person struct {
	Email    string    `json:"email"`
	Org      string    `json:"org"`
	Role     Role      `json:"role"`
	Created  time.Time `json:"created"`
	Verified time.Time `json:"verified,omitzero"`
}

// Org is an organisation as the front door knows it. Its work is in its own
// directory and its own server, and this is only who it is and who made it.
type Org struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Admin string `json:"admin"`
	// Domain is the company's domain when the admin signed up from one, and is
	// empty for an address at a free mail provider.
	Domain  string    `json:"domain,omitempty"`
	Created time.Time `json:"created"`
}

// Limits are the two quotas an organisation moves between, and the settings that
// make them.
type Limits struct {
	Trial, Verified quota.Quota
}

// DefaultLimits are what a sign-up may run before and after it is verified.
func DefaultLimits() Limits {
	return Limits{
		Trial:    quota.Quota{Jobs: 3, Concurrent: 1, Members: 1, Computers: 1, Note: "Confirm the admin's email address to raise this limit."},
		Verified: quota.Quota{Jobs: 200, Monthly: true, Concurrent: 4, Invites: true, Members: 4, Computers: 4},
	}
}

// Trial says a person's organisation is still on the small quota.
func (p Person) Trial() bool { return p.Verified.IsZero() }
