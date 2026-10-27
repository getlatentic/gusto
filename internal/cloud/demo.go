package cloud

import (
	"errors"
	"time"

	"github.com/getlatentic/gusto/internal/quota"
)

// DemoSettings make the front door offer a demo: an organization that a visitor starts without signing up, that
// has a time, and that is deleted when the time is up. A demo is offered when Length is set.
type DemoSettings struct {
	// Length is how long a demo lasts from when its server is ready, and Extend is what the one extension adds.
	Length, Extend time.Duration
	// Patience is how long a demo may wait to be ready before it is dropped, which is when nothing could start it.
	Patience time.Duration
	// MaxOrgs is the most demos at once.
	MaxOrgs int
	// BookURL is where a visitor books a call, which is shown when the demo ends.
	BookURL string
	// Limits is what a demo may run.
	Limits quota.Quota
	// VideoURL is a video the landing page shows: a file (.mp4 or .webm) or a page that may be embedded, at https.
	VideoURL string
	// PerDay is the most demos started in a day, in all, which bounds what a day of demos can cost. Zero is no limit.
	PerDay int
	// PerPerson is the most demos one address may start in a day, and PerCompany the most that one company's addresses
	// may: a day is the last twenty-four hours. A visitor who has had them is asked to book a call. Zero is no limit.
	PerPerson, PerCompany int
}

// Enabled says the front door offers a demo.
func (d DemoSettings) Enabled() bool { return d.Length > 0 }

func (d DemoSettings) patience() time.Duration {
	if d.Patience > 0 {
		return d.Patience
	}
	return 15 * time.Minute
}

// DefaultDemoLimits is what a demo may run: enough for a person to see agents do work, and not enough to cost much.
func DefaultDemoLimits() quota.Quota {
	return quota.Quota{Jobs: 30, Concurrent: 2, Invites: true, Members: 2, Computers: 1, Note: "This is a demo, which is removed when its time is up."}
}

// Demo is what makes an organization a demo. Ready is when its server first answered, which is when its time
// starts, so a visitor who waits for a cell to start is not charged for the wait.
type Demo struct {
	Name     string    `json:"name"`
	Company  string    `json:"company"`
	Ready    time.Time `json:"ready,omitzero"`
	Expires  time.Time `json:"expires,omitzero"`
	Extended bool      `json:"extended,omitempty"`
	// Stopped is a demo whose visitor ended it before its time was up.
	Stopped bool `json:"stopped,omitempty"`
}

// Lead is a visitor who started a demo, kept after the demo is gone.
type Lead struct {
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Company   string    `json:"company"`
	Requested time.Time `json:"requested"`
	Ready     time.Time `json:"ready,omitzero"`
	Ended     time.Time `json:"ended,omitzero"`
	Extended  bool      `json:"extended,omitempty"`
	// Reason is why the demo ended: its time was up, or it was never ready.
	Reason string `json:"reason,omitempty"`
}

// The refusals of a demo.
var (
	ErrDemoFull      = errors.New("all the demos are in use")
	ErrNeedCompany   = errors.New("say which company you are with")
	ErrDemoNeedsName = errors.New("say who you are")
	ErrNoExtension   = errors.New("this demo cannot be extended any more")
	ErrNotDemo       = errors.New("this organization is not a demo")
	ErrDemoPaused    = errors.New("demos are paused")
	ErrDemoDaily     = errors.New("today's demos are all taken")
	ErrDemoRepeat    = errors.New("that address has had its demos for today")
)
