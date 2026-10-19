package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

const slugAttempts = 5

// StartDemo makes a person and the demo organization they administer. A refusal leaves nothing behind.
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
	ctx := context.Background()
	var person Person
	var org Org
	err = s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
		person, org, err = s.startDemoIn(ctx, tx, name, address, title)
		return err
	})
	if err != nil {
		return Person{}, Org{}, err
	}
	return person, org, nil
}

// startDemoIn checks the day's counts and makes the demo in one step under the lock all demos take, so
// that two front doors cannot both take the last place.
func (s *Store) startDemoIn(ctx context.Context, tx dbkit.Tx, name, address, title string) (Person, Org, error) {
	if err := s.lock(ctx, tx, "demos"); err != nil {
		return Person{}, Org{}, err
	}
	if _, found, err := fetchPerson(ctx, tx, address); err != nil || found {
		if err == nil {
			err = ErrEmailTaken
		}
		return Person{}, Org{}, err
	}
	if err := s.demoAllowed(ctx, tx, address); err != nil {
		return Person{}, Org{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	for range slugAttempts {
		slug, err := demoSlug(title)
		if err != nil {
			return Person{}, Org{}, err
		}
		person := Person{Email: address, Org: slug, Role: Admin, Created: now}
		org := Org{Slug: slug, Name: title, Admin: address, Created: now, Demo: &Demo{Name: name, Company: title}}
		err = s.create(ctx, tx, person, org, s.set.Demo.Limits)
		if errors.Is(err, ErrSlugTaken) {
			continue
		}
		if err != nil {
			return Person{}, Org{}, err
		}
		return person, org, s.putLead(ctx, tx, Lead{Slug: slug, Name: name, Email: address, Company: title, Requested: now})
	}
	return Person{}, Org{}, ErrSlugTaken
}

// demoAllowed is a refusal when the address, the company, the day or the host has had as many demos as it may.
func (s *Store) demoAllowed(ctx context.Context, q dbkit.Querier, address string) error {
	limits := s.DemoLimits()
	if err := s.withinHabit(ctx, q, address, limits); err != nil {
		return err
	}
	if started, err := s.startedToday(ctx, q); err != nil || (limits.PerDay > 0 && started >= limits.PerDay) {
		if err == nil {
			err = ErrDemoDaily
		}
		return err
	}
	if live, err := count(ctx, q, `SELECT count(*) FROM orgs WHERE is_demo`); err != nil || live >= limits.MaxOrgs {
		if err == nil {
			err = ErrDemoFull
		}
		return err
	}
	return nil
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
