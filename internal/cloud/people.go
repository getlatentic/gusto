package cloud

import "time"

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
	Slug    string    `json:"slug"`
	Name    string    `json:"name"`
	Admin   string    `json:"admin"`
	Created time.Time `json:"created"`
}

// Quota is what an organisation may run, and is written where the
// organisation's own server reads it. Zero is none.
type Quota struct {
	// Jobs is how many jobs may be started: in all for a trial, and in a month
	// once verified.
	Jobs int `json:"jobs"`
	// Monthly says Jobs is counted for the month and not in all.
	Monthly bool `json:"monthly,omitempty"`
	// Concurrent is how many jobs may run at once.
	Concurrent int `json:"concurrent"`
	// Invites says whether colleagues may be invited.
	Invites bool `json:"invites"`
}

// Limits are the two quotas an organisation moves between, and the settings that
// make them.
type Limits struct {
	Trial, Verified Quota
}

// DefaultLimits are what a sign-up may run before and after it is verified.
func DefaultLimits() Limits {
	return Limits{
		Trial:    Quota{Jobs: 3, Concurrent: 1},
		Verified: Quota{Jobs: 200, Monthly: true, Concurrent: 4, Invites: true},
	}
}

// Trial says a person's organisation is still on the small quota.
func (p Person) Trial() bool { return p.Verified.IsZero() }
