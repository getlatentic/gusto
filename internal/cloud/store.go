package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/quota"
	"golang.org/x/net/publicsuffix"
)

// Settings are what an operator chooses about sign-ups.
type Settings struct {
	// AllowFreeMail lets addresses at free mail providers sign up, which a
	// demonstration wants and a service for companies does not.
	AllowFreeMail bool
	// RefusedDomains are more domains to turn away, beside the built-in ones.
	RefusedDomains []string
	// PerDomain is how many organisations one email domain may make.
	PerDomain int
	Limits    Limits
	// Demo, when it is set, makes the front door offer a demo.
	Demo DemoSettings
}

// Store is the front door's accounts, kept as files under one directory. A file
// is written by renaming a finished one into place, and a name that must be
// unique is claimed by creating its file, so two sign-ups cannot take one.
// One front door owns a directory: the counts it keeps are read under its lock.
type Store struct {
	root    string
	set     Settings
	refused map[string]bool
	mu      sync.Mutex
	now     func() time.Time
}

// Open makes the directories a store keeps its files in, and sets aside what a
// sign-up that stopped part way left: an organisation's directory with no record.
func Open(root string, set Settings) (*Store, error) {
	if set.PerDomain <= 0 {
		set.PerDomain = 3
	}
	if set.Limits == (Limits{}) {
		set.Limits = DefaultLimits()
	}
	if set.Demo.Enabled() && set.Demo.Limits == (quota.Quota{}) {
		set.Demo.Limits = DefaultDemoLimits()
	}
	for _, dir := range []string{"", "people", "orgs", "tokens", "outbox", "demos"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			return nil, err
		}
	}
	s := &Store{root: root, set: set, refused: setOf(set.RefusedDomains...), now: time.Now}
	return s, s.clearUnfinished()
}

// Attach is a second program's view of a store another program runs. It changes nothing in it when it opens, since a
// sign-up that looks unfinished to it may be one the other program is part way through.
func Attach(root string, set Settings) (*Store, error) {
	if _, err := os.Stat(filepath.Join(root, "orgs")); err != nil {
		return nil, fmt.Errorf("%s holds no organisations: %w", root, err)
	}
	return &Store{root: root, set: set, refused: setOf(set.RefusedDomains...), now: time.Now}, nil
}

func (s *Store) clearUnfinished() error {
	orgs, err := os.ReadDir(filepath.Join(s.root, "orgs"))
	if err != nil {
		return err
	}
	for _, entry := range orgs {
		if _, err := os.Stat(filepath.Join(s.root, "orgs", entry.Name(), "org.json")); missing(err) {
			if err := os.RemoveAll(filepath.Join(s.root, "orgs", entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// Root is the directory the store keeps its files in.
func (s *Store) Root() string { return s.root }

func (s *Store) personFile(email string) string {
	sum := sha256.Sum256([]byte(email))
	return filepath.Join(s.root, "people", hex.EncodeToString(sum[:16])+".json")
}

// OrgDir is where an organisation's own files are, and DataDir the directory its
// gusto server is given. Only the organisation's server and this process reach
// them, and they are not readable by anyone else on the machine.
func (s *Store) OrgDir(slug string) string  { return filepath.Join(s.root, "orgs", slug) }
func (s *Store) DataDir(slug string) string { return filepath.Join(s.OrgDir(slug), "data") }

// Signup is what a person gives to start an organisation.
type Signup struct {
	Email, OrgName string
	// Slug is the organisation's address. Left out, it is made from the name.
	Slug string
}

// SignUp makes a person, the organisation they administer and the organisation's
// directory, on the trial quota. A refusal leaves nothing behind.
func (s *Store) SignUp(in Signup) (Person, Org, error) {
	email, err := NormalEmail(in.Email)
	if err != nil {
		return Person{}, Org{}, err
	}
	if err := workEmail(email, s.set.AllowFreeMail, s.refused); err != nil {
		return Person{}, Org{}, err
	}
	domain := s.companyDomain(email)

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, found, err := s.person(email); err != nil || found {
		if err == nil {
			err = ErrEmailTaken
		}
		return Person{}, Org{}, err
	}
	existing, pending, err := s.orgsAt(domain)
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
	return s.createAt(email, name, slug, domain, derived, now)
}

// identify is the organisation's name and address. The first at a company's
// domain has the address that domain gives, so no company is put behind another
// by arriving later. A company can have more than one organisation, a subsidiary or
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

// orgsAt is how many organisations a company's domain has, and how many of them
// have an admin who has not yet confirmed their address. It is the second that
// is limited: a company may have as many organisations as it has real people to
// make them, and not more than a few that nobody has answered for.
func (s *Store) orgsAt(domain string) (existing, pending int, err error) {
	if domain == "" {
		return 0, 0, nil
	}
	entries, err := os.ReadDir(filepath.Join(s.root, "orgs"))
	if err != nil {
		return 0, 0, err
	}
	for _, entry := range entries {
		var org Org
		if readFile(filepath.Join(s.root, "orgs", entry.Name(), "org.json"), &org) != nil || org.Domain != domain {
			continue
		}
		existing++
		if admin, found, err := s.person(org.Admin); err == nil && found && admin.Trial() {
			pending++
		}
	}
	return existing, pending, nil
}

// createAt makes the organisation at the address. An address that came from the
// company's domain and is taken means the company has an organisation now, made
// a moment ago, so this one is asked for a name of its own.
func (s *Store) createAt(email, name, slug, domain string, derived bool, now time.Time) (Person, Org, error) {
	person := Person{Email: email, Org: slug, Role: Admin, Created: now}
	org := Org{Slug: slug, Name: name, Admin: email, Domain: domain, Created: now}
	err := s.create(person, org)
	if derived && errors.Is(err, ErrSlugTaken) {
		err = ErrSameCompany
	}
	return person, org, err
}

// nameFor is the organisation's name: the one given, or the one the company's
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

// create claims the slug by making its directory, then the address by linking
// its file, and undoes both if anything after the claims fails.
func (s *Store) create(person Person, org Org) error {
	dir := s.OrgDir(org.Slug)
	if err := os.Mkdir(dir, 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrSlugTaken
		}
		return err
	}
	undo := func(err error) error {
		os.RemoveAll(dir)
		return err
	}
	if err := os.Mkdir(s.DataDir(org.Slug), 0o700); err != nil {
		return undo(err)
	}
	if err := createFile(s.personFile(person.Email), person, 0o600); err != nil {
		if errors.Is(err, fs.ErrExist) {
			err = ErrEmailTaken
		}
		return undo(err)
	}
	if err := s.setQuota(org.Slug, s.set.Limits.Trial); err != nil {
		os.Remove(s.personFile(person.Email))
		return undo(err)
	}
	// The organisation's record is written last: a directory without one is what
	// Open sets aside.
	if err := writeFile(filepath.Join(dir, "org.json"), org, 0o600); err != nil {
		os.Remove(s.personFile(person.Email))
		return undo(err)
	}
	return nil
}

// Person is the account at an address.
func (s *Store) Person(email string) (Person, bool, error) {
	email, err := NormalEmail(email)
	if err != nil {
		return Person{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.person(email)
}

func (s *Store) person(email string) (Person, bool, error) {
	var person Person
	err := readFile(s.personFile(email), &person)
	if missing(err) {
		return Person{}, false, nil
	}
	return person, err == nil, err
}

// Org is an organisation by its address.
func (s *Store) Org(slug string) (Org, bool, error) {
	if ValidSlug(slug) != nil {
		return Org{}, false, nil
	}
	var org Org
	err := readFile(filepath.Join(s.OrgDir(slug), "org.json"), &org)
	if missing(err) {
		return Org{}, false, nil
	}
	return org, err == nil, err
}

// Verify records that a person read the mail sent to them, and lifts their
// organisation's quota when they administer it.
func (s *Store) Verify(email string) (Person, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	person, found, err := s.person(email)
	if err != nil || !found {
		if err == nil {
			err = fs.ErrNotExist
		}
		return Person{}, err
	}
	if !person.Verified.IsZero() {
		return person, nil
	}
	person.Verified = s.now().UTC().Truncate(time.Second)
	if err := writeFile(s.personFile(email), person, 0o600); err != nil {
		return Person{}, err
	}
	if person.Role != Admin {
		return person, nil
	}
	return person, s.setQuota(person.Org, s.set.Limits.Verified)
}

// Quota is what an organisation may run now.
func (s *Store) Quota(slug string) (quota.Quota, error) {
	var q quota.Quota
	return q, readFile(s.QuotaFile(slug), &q)
}

// Computers is the organisation's record of its computers, which its server keeps
// and this reads.
func (s *Store) Computers(slug string) (*computer.Registry, error) {
	return computer.Open(filepath.Join(s.DataDir(slug), "computers"), slug)
}

// QuotaFile is where the organisation's server reads its limits.
func (s *Store) QuotaFile(slug string) string { return filepath.Join(s.DataDir(slug), "quota.json") }

func (s *Store) setQuota(slug string, q quota.Quota) error { return quota.Write(s.QuotaFile(slug), q) }
