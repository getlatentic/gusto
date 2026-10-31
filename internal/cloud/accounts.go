package cloud

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
	"golang.org/x/net/publicsuffix"
)

// Signup is what a person gives to start an organization.
type Signup struct {
	Email, OrgName string
	// Slug is the organization's address. Left out, it is made from the name.
	Slug string
}

// SignUp makes a person, the organization they administer and the organization's
// directory, on the trial quota. A refusal leaves nothing behind.
func (s *Store) SignUp(in Signup) (Person, Org, error) {
	email, err := NormalEmail(in.Email)
	if err != nil {
		return Person{}, Org{}, err
	}
	if err := workEmail(email, s.set.AllowFreeMail, s.refused); err != nil {
		return Person{}, Org{}, err
	}
	ctx := context.Background()
	var person Person
	var org Org
	err = s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
		person, org, err = s.signUpIn(ctx, tx, in, email, s.companyDomain(email))
		return err
	})
	if err != nil {
		return Person{}, Org{}, err
	}
	return person, org, nil
}

// MaySignUp says whether an organization may be made with the address, as far as the address alone says.
func (s *Store) MaySignUp(email string) error {
	normal, err := NormalEmail(email)
	if err != nil {
		return err
	}
	return workEmail(normal, s.set.AllowFreeMail, s.refused)
}

func (s *Store) signUpIn(ctx context.Context, tx dbkit.Tx, in Signup, email, domain string) (Person, Org, error) {
	if found, err := hasAccount(ctx, tx, email); err != nil || found {
		if err == nil {
			err = ErrEmailTaken
		}
		return Person{}, Org{}, err
	}
	if domain != "" {
		if err := s.lock(ctx, tx, "domain:"+domain); err != nil {
			return Person{}, Org{}, err
		}
	}
	existing, pending, err := orgsAt(ctx, tx, domain)
	if err != nil {
		return Person{}, Org{}, err
	}
	if domain != "" && pending >= s.set.PerDomain {
		return Person{}, Org{}, ErrDomainFull
	}
	name, slug, derived, err := s.identify(in, email, domain, existing)
	if err != nil {
		return Person{}, Org{}, err
	}
	if err := ValidSlug(slug); err != nil {
		return Person{}, Org{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	person := Person{Email: email, Org: slug, Role: Admin, Created: now}
	org := Org{Slug: slug, Name: name, Admin: email, Domain: domain, Created: now}
	err = s.create(ctx, tx, person, org, s.set.Limits.Trial)
	if derived && errors.Is(err, ErrSlugTaken) {
		err = ErrSameCompany
	}
	return person, org, err
}

// identify is the organization's name and address. The first at a company's
// domain has the address that domain gives, so no company is put behind another
// by arriving later. A company can have more than one organization, a subsidiary or
// a team, and each after the first is named by whoever makes it and has an address
// from that name. An address at a free mail provider has only a name to go on.
func (s *Store) identify(in Signup, email, domain string, existing int) (name, slug string, derived bool, err error) {
	given := strings.TrimSpace(in.OrgName) != ""
	slug = strings.ToLower(strings.TrimSpace(in.Slug))
	switch {
	case slug != "":
		name, err = s.nameFor(in.OrgName, email)
	case domain == "":
		if name, err = s.nameFor(in.OrgName, email); err == nil {
			slug = SlugFor(name)
		}
	case existing == 0:
		if name, err = s.nameFor(in.OrgName, email); err == nil {
			slug, derived = AddressFor(domain), true
		}
	case !given:
		err = ErrSameCompany
	default:
		if name, err = OrgName(in.OrgName); err == nil {
			slug = SlugFor(name)
		}
	}
	return name, slug, derived, err
}

// nameFor is the organization's name: the one given, or the one the company's
// domain suggests.
func (s *Store) nameFor(given, email string) (string, error) {
	if strings.TrimSpace(given) != "" {
		return OrgName(given)
	}
	suggested, err := FromDomain(DomainOf(email))
	if err != nil {
		return "", err
	}
	return OrgName(suggested)
}

// companyDomain is the company's domain an address is at: the registrable part,
// so ada@mail.acme.com and bob@acme.com are one company's. It is empty at a free
// mail provider, which is nobody's to claim.
func (s *Store) companyDomain(email string) string {
	domain := DomainOf(email)
	if freeMail[domain] {
		return ""
	}
	if registrable, err := publicsuffix.EffectiveTLDPlusOne(domain); err == nil {
		return registrable
	}
	return domain
}
