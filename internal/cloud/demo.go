package cloud

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/quota"
)

// DemoSettings make the front door offer a demo: an organisation that a visitor starts without signing up, that
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
	return quota.Quota{Jobs: 30, Concurrent: 2, Members: 1, Computers: 1, Note: "This is a demo, which is removed when its time is up."}
}

// Demo is what makes an organisation a demo. Ready is when its server first answered, which is when its time
// starts, so a visitor who waits for a cell to start is not charged for the wait.
type Demo struct {
	Name     string    `json:"name"`
	Company  string    `json:"company"`
	Ready    time.Time `json:"ready,omitzero"`
	Expires  time.Time `json:"expires,omitzero"`
	Extended bool      `json:"extended,omitempty"`
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
	ErrDemoPaused    = errors.New("demos are paused")
	ErrDemoDaily     = errors.New("today's demos are all taken")
)

func (s *Store) leadFile(slug string) string { return filepath.Join(s.root, "demos", slug+".json") }

// StartDemo makes a person and the demo organisation they administer. A refusal leaves nothing behind.
func (s *Store) StartDemo(name, email, company string) (Person, Org, error) {
	name = strings.TrimSpace(name)
	switch {
	case !s.set.Demo.Enabled():
		return Person{}, Org{}, errors.New("this host offers no demo")
	case s.DemosPaused():
		return Person{}, Org{}, ErrDemoPaused
	case name == "" || len(name) > 80:
		return Person{}, Org{}, ErrDemoNeedsName
	case strings.TrimSpace(company) == "":
		return Person{}, Org{}, ErrNeedCompany
	}
	address, err := NormalEmail(email)
	if err != nil {
		return Person{}, Org{}, err
	}
	if err := workEmail(address, s.set.AllowFreeMail, s.refused); err != nil {
		return Person{}, Org{}, err
	}
	title, err := OrgName(company)
	if err != nil {
		return Person{}, Org{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, found, err := s.person(address); err != nil || found {
		if err == nil {
			err = ErrEmailTaken
		}
		return Person{}, Org{}, err
	}
	if started, err := s.startedToday(); err != nil || (s.set.Demo.PerDay > 0 && started >= s.set.Demo.PerDay) {
		if err == nil {
			err = ErrDemoDaily
		}
		return Person{}, Org{}, err
	}
	if live, err := s.demoCount(); err != nil || live >= max(s.set.Demo.MaxOrgs, 1) {
		if err == nil {
			err = ErrDemoFull
		}
		return Person{}, Org{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	person := Person{Email: address, Role: Admin, Created: now}
	for range 5 {
		slug, err := demoSlug(title)
		if err != nil {
			return Person{}, Org{}, err
		}
		person.Org = slug
		org := Org{Slug: slug, Name: title, Admin: address, Created: now, Demo: &Demo{Name: name, Company: title}}
		err = s.create(person, org)
		if errors.Is(err, ErrSlugTaken) {
			continue
		}
		if err != nil {
			return Person{}, Org{}, err
		}
		if err := s.setQuota(slug, s.set.Demo.Limits); err != nil {
			s.removeOrg(slug)
			return Person{}, Org{}, err
		}
		if err := writeFile(s.leadFile(slug), Lead{Slug: slug, Name: name, Email: address, Company: title, Requested: now}, 0o600); err != nil {
			s.removeOrg(slug)
			return Person{}, Org{}, err
		}
		return person, org, nil
	}
	return Person{}, Org{}, ErrSlugTaken
}

// demoSlug is an address for a demo: the company's name, and four letters that keep two visitors from one company apart.
func demoSlug(company string) (string, error) {
	random := make([]byte, 2)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	base := strings.Trim(slugify(company), "-")
	if len(base) > 18 {
		base = strings.Trim(base[:18], "-")
	}
	slug := "demo-" + hex.EncodeToString(random)
	if base != "" {
		slug = "demo-" + base + "-" + hex.EncodeToString(random)
	}
	return slug, ValidSlug(slug)
}

// demoCount is the demos there are.
func (s *Store) demoCount() (int, error) {
	orgs, err := s.Orgs()
	count := 0
	for _, org := range orgs {
		if org.Demo != nil {
			count++
		}
	}
	return count, err
}

// updateOrg changes an organisation's record.
func (s *Store) updateOrg(slug string, change func(*Org) error) (Org, error) {
	path := filepath.Join(s.OrgDir(slug), "org.json")
	var org Org
	if err := readFile(path, &org); err != nil {
		return Org{}, err
	}
	if err := change(&org); err != nil {
		return org, err
	}
	return org, writeFile(path, org, 0o600)
}

// DemoReady starts a demo's time, when it is not started. It is asked each time the organisation is found ready and
// starts the time once.
func (s *Store) DemoReady(slug string) (Demo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	org, err := s.updateOrg(slug, func(org *Org) error {
		if org.Demo == nil {
			return errors.New("that is not a demo")
		}
		if org.Demo.Ready.IsZero() {
			now := s.now().UTC().Truncate(time.Second)
			org.Demo.Ready, org.Demo.Expires = now, now.Add(s.set.Demo.Length)
		}
		return nil
	})
	if err != nil {
		return Demo{}, err
	}
	var lead Lead
	if readFile(s.leadFile(slug), &lead) == nil && lead.Ready.IsZero() {
		lead.Ready = org.Demo.Ready
		writeFile(s.leadFile(slug), lead, 0o600)
	}
	return *org.Demo, nil
}

// ExtendDemo gives a demo the one extension it may have, while it still has time.
func (s *Store) ExtendDemo(slug string) (Demo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	org, err := s.updateOrg(slug, func(org *Org) error {
		demo := org.Demo
		if demo == nil || demo.Extended || s.set.Demo.Extend <= 0 || demo.Expires.IsZero() || !s.now().Before(demo.Expires) {
			return ErrNoExtension
		}
		demo.Expires, demo.Extended = demo.Expires.Add(s.set.Demo.Extend), true
		return nil
	})
	if err != nil {
		return Demo{}, err
	}
	var lead Lead
	if readFile(s.leadFile(slug), &lead) == nil {
		lead.Extended = true
		writeFile(s.leadFile(slug), lead, 0o600)
	}
	return *org.Demo, nil
}

// DemoLeft is a demo's record and how long it has, and whether the organisation is a demo at all. A demo that is not
// ready has its whole time still, which is not counting down.
func (s *Store) DemoLeft(slug string) (Demo, time.Duration, bool) {
	org, found, err := s.Org(slug)
	if err != nil || !found || org.Demo == nil {
		return Demo{}, 0, false
	}
	if org.Demo.Expires.IsZero() {
		return *org.Demo, s.set.Demo.Length, true
	}
	return *org.Demo, max(org.Demo.Expires.Sub(s.now()), 0), true
}

// DemosDue are the demos whose time is up, or that were never ready and have waited as long as they may.
func (s *Store) DemosDue() (map[string]string, error) {
	orgs, err := s.Orgs()
	if err != nil {
		return nil, err
	}
	now := s.now()
	due := map[string]string{}
	for _, org := range orgs {
		switch {
		case org.Demo == nil:
		case !org.Demo.Expires.IsZero() && !now.Before(org.Demo.Expires):
			due[org.Slug] = "its time was up"
		case org.Demo.Ready.IsZero() && now.Sub(org.Created) > s.set.Demo.patience():
			due[org.Slug] = "it was never ready"
		}
	}
	return due, nil
}

// EndDemo deletes a demo: its people, its data and its record, and keeps the lead with why it ended. The
// organisation's server and computers are stopped before this, since nothing here reaches them.
func (s *Store) EndDemo(slug, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if org, found, err := s.Org(slug); err != nil || !found || org.Demo == nil {
		if err == nil {
			err = errors.New("that is not a demo")
		}
		return err
	}
	var lead Lead
	if readFile(s.leadFile(slug), &lead) == nil {
		lead.Ended, lead.Reason = s.now().UTC().Truncate(time.Second), reason
		if err := writeFile(s.leadFile(slug), lead, 0o600); err != nil {
			return err
		}
	}
	return s.removeOrg(slug)
}

// removeOrg takes away the people of an organisation and then the organisation, the record last, so a failure part way
// leaves an organisation that is found and tried again.
func (s *Store) removeOrg(slug string) error {
	people, err := os.ReadDir(filepath.Join(s.root, "people"))
	if err != nil {
		return err
	}
	for _, entry := range people {
		var person Person
		if readFile(filepath.Join(s.root, "people", entry.Name()), &person) == nil && person.Org == slug {
			if err := os.Remove(filepath.Join(s.root, "people", entry.Name())); err != nil {
				return err
			}
		}
	}
	return os.RemoveAll(s.OrgDir(slug))
}

// LeadOf is the lead a demo made, found after the demo is gone.
func (s *Store) LeadOf(slug string) (Lead, bool) {
	var lead Lead
	if ValidSlug(slug) != nil || readFile(s.leadFile(slug), &lead) != nil {
		return Lead{}, false
	}
	return lead, true
}

// pauseFile is the file whose presence pauses demos, which an operator makes or removes without restarting anything.
func (s *Store) pauseFile() string { return filepath.Join(s.root, "demos.off") }

// DemosPaused says an operator has paused demos. Demos that are running go on.
func (s *Store) DemosPaused() bool {
	_, err := os.Stat(s.pauseFile())
	return err == nil
}

// startedToday is how many demos were asked for since midnight UTC.
func (s *Store) startedToday() (int, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "demos"))
	if err != nil {
		return 0, err
	}
	midnight := s.now().UTC().Truncate(24 * time.Hour)
	count := 0
	for _, entry := range entries {
		var lead Lead
		if readFile(filepath.Join(s.root, "demos", entry.Name()), &lead) == nil && !lead.Requested.Before(midnight) {
			count++
		}
	}
	return count, nil
}
