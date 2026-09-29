package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/haas/internal/computer"
	"github.com/getlatentic/haas/internal/quota"
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
	for _, dir := range []string{"", "people", "orgs", "tokens", "outbox"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			return nil, err
		}
	}
	s := &Store{root: root, set: set, refused: setOf(set.RefusedDomains...), now: time.Now}
	return s, s.clearUnfinished()
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
// haas server is given. Only the organisation's server and this process reach
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
	name, err := OrgName(in.OrgName)
	if err != nil {
		return Person{}, Org{}, err
	}
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	if slug == "" {
		slug = SlugFor(name)
	}
	if err := ValidSlug(slug); err != nil {
		return Person{}, Org{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, found, err := s.person(email); err != nil || found {
		if err == nil {
			err = ErrEmailTaken
		}
		return Person{}, Org{}, err
	}
	if made, err := s.orgsOfDomain(DomainOf(email)); err != nil {
		return Person{}, Org{}, err
	} else if made >= s.set.PerDomain {
		return Person{}, Org{}, ErrDomainFull
	}
	now := s.now().UTC().Truncate(time.Second)
	person := Person{Email: email, Org: slug, Role: Admin, Created: now}
	org := Org{Slug: slug, Name: name, Admin: email, Created: now}
	return person, org, s.create(person, org)
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

func (s *Store) orgsOfDomain(domain string) (int, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "orgs"))
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		var org Org
		if err := readFile(filepath.Join(s.root, "orgs", entry.Name(), "org.json"), &org); err != nil {
			continue
		}
		if DomainOf(org.Admin) == domain {
			count++
		}
	}
	return count, nil
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
	if person.Role == Admin {
		return person, s.setQuota(person.Org, s.set.Limits.Verified)
	}
	return person, nil
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
