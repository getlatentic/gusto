package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Refusals a member's invitation or removal can meet.
var (
	ErrNotAdmin     = errors.New("only an admin can do that")
	ErrNoInvites    = errors.New("this organisation cannot invite people until its admin has confirmed their email address")
	ErrFull         = errors.New("this organisation has as many people as it may")
	ErrCannotInvite = errors.New("that address cannot be invited")
	ErrLastAdmin    = errors.New("an organisation keeps its last admin")
	ErrNoMember     = errors.New("nobody by that address is in this organisation")
)

// Invitation is an invitation that has not been answered.
type Invitation struct {
	Email   string    `json:"email"`
	Role    Role      `json:"role"`
	By      string    `json:"by"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
	// Link is the digest of the link's secret, which is how a cancelled
	// invitation's link is taken away without anyone holding the secret.
	Link string `json:"link"`
}

// InviteLife is how long an invitation can be accepted.
const InviteLife = 7 * 24 * time.Hour

func (s *Store) invitesDir(slug string) string { return filepath.Join(s.OrgDir(slug), "invites") }

func (s *Store) invitationFile(slug, email string) string {
	sum := sha256.Sum256([]byte(email))
	return filepath.Join(s.invitesDir(slug), hex.EncodeToString(sum[:16])+".json")
}

// Members is the people in an organisation, its admins first and then in the order
// they joined.
func (s *Store) Members(slug string) ([]Person, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.members(slug)
}

func (s *Store) members(slug string) ([]Person, error) {
	names, err := readDir(filepath.Join(s.root, "people"))
	if err != nil {
		return nil, err
	}
	var members []Person
	for _, name := range names {
		var person Person
		if readFile(filepath.Join(s.root, "people", name), &person) == nil && person.Org == slug {
			members = append(members, person)
		}
	}
	sort.SliceStable(members, func(a, b int) bool {
		if (members[a].Role == Admin) != (members[b].Role == Admin) {
			return members[a].Role == Admin
		}
		return members[a].Created.Before(members[b].Created)
	})
	return members, nil
}

// Invitations is what an organisation has sent that nobody has answered, oldest
// first. One that ran out is not listed.
func (s *Store) Invitations(slug string) ([]Invitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.invitations(slug)
}

func (s *Store) invitations(slug string) ([]Invitation, error) {
	names, err := readDir(s.invitesDir(slug))
	if missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var open []Invitation
	for _, name := range names {
		var invitation Invitation
		if readFile(filepath.Join(s.invitesDir(slug), name), &invitation) == nil && s.now().Before(invitation.Expires) {
			open = append(open, invitation)
		}
	}
	sort.Slice(open, func(a, b int) bool { return open[a].Created.Before(open[b].Created) })
	return open, nil
}

// Invite makes an invitation for an address, from an admin. It is refused unless the
// organisation may invite, has room for one more person counting those already
// invited, and the address is not already somebody's here or elsewhere. The refusal
// for an address that has an account says nothing of whose, so the form is not a way
// to find out.
func (s *Store) Invite(slug string, by Person, address string) (secret string, err error) {
	email, err := NormalEmail(address)
	if err != nil {
		return "", err
	}
	if err := workEmail(email, true, s.refused); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if by.Org != slug || by.Role != Admin {
		return "", ErrNotAdmin
	}
	quota, err := s.Quota(slug)
	if err != nil {
		return "", err
	}
	if !quota.Invites {
		return "", ErrNoInvites
	}
	if _, found, err := s.person(email); err != nil || found {
		if err == nil {
			err = ErrCannotInvite
		}
		return "", err
	}
	if err := s.roomFor(slug, quota.Members, email); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.invitesDir(slug), 0o700); err != nil {
		return "", err
	}
	now := s.now().UTC().Truncate(time.Second)
	secret, err = s.makeLink(link{Email: email, Purpose: Invite, Expires: now.Add(InviteLife), Org: slug, Role: Runner, By: by.Email})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(secret))
	invitation := Invitation{Email: email, Role: Runner, By: by.Email, Created: now, Expires: now.Add(InviteLife), Link: hex.EncodeToString(sum[:])}
	// Inviting an address again replaces the earlier invitation, and takes its link away.
	if earlier := s.readInvitation(slug, email); earlier.Link != "" {
		os.Remove(filepath.Join(s.root, "tokens", earlier.Link+".json"))
	}
	if err := writeFile(s.invitationFile(slug, email), invitation, 0o600); err != nil {
		return "", err
	}
	s.publishRoster(slug)
	return secret, nil
}

// roomFor is a refusal when one more person would pass the number the organisation may
// have. People and the invitations that are open both count, and an address that is
// already invited does not count twice.
func (s *Store) roomFor(slug string, most int, email string) error {
	members, err := s.members(slug)
	if err != nil {
		return err
	}
	open, err := s.invitations(slug)
	if err != nil {
		return err
	}
	have := len(members)
	for _, invitation := range open {
		if invitation.Email != email {
			have++
		}
	}
	if have >= most {
		return ErrFull
	}
	return nil
}

func (s *Store) readInvitation(slug, email string) Invitation {
	var invitation Invitation
	readFile(s.invitationFile(slug, email), &invitation)
	return invitation
}

// Cancel takes an invitation back, and its link with it.
func (s *Store) Cancel(slug string, by Person, address string) error {
	email, err := NormalEmail(address)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if by.Org != slug || by.Role != Admin {
		return ErrNotAdmin
	}
	if invitation := s.readInvitation(slug, email); invitation.Link != "" {
		os.Remove(filepath.Join(s.root, "tokens", invitation.Link+".json"))
	}
	if err := os.Remove(s.invitationFile(slug, email)); err != nil && !missing(err) {
		return err
	}
	s.publishRoster(slug)
	return nil
}

// Accept makes a person of an invitation's link. They are verified, because they read
// the mail the link was in, and they are a runner. The room is checked again, since
// the organisation may have filled while the invitation was out.
func (s *Store) Accept(secret string) (Person, Org, error) {
	invited, err := s.useLink(secret, Invite)
	if err != nil {
		return Person{}, Org{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	org, found, err := s.Org(invited.Org)
	if err != nil || !found {
		return Person{}, Org{}, ErrLink
	}
	os.Remove(s.invitationFile(invited.Org, invited.Email))
	quota, err := s.Quota(org.Slug)
	if err != nil {
		return Person{}, Org{}, err
	}
	members, err := s.members(org.Slug)
	if err != nil {
		return Person{}, Org{}, err
	}
	if len(members) >= quota.Members {
		return Person{}, Org{}, ErrFull
	}
	now := s.now().UTC().Truncate(time.Second)
	person := Person{Email: invited.Email, Org: org.Slug, Role: invited.Role, Created: now, Verified: now, By: invited.By}
	if err := createFile(s.personFile(person.Email), person, 0o600); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Person{}, Org{}, ErrEmailTaken
		}
		return Person{}, Org{}, err
	}
	s.publishRoster(org.Slug)
	return person, org, nil
}

// Remove takes a person out of an organisation. An admin may remove anybody but
// themselves and the last admin, so an organisation is never left with nobody to run it.
func (s *Store) Remove(slug string, by Person, address string) error {
	email, err := NormalEmail(address)
	if err != nil {
		return ErrNoMember
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if by.Org != slug || by.Role != Admin {
		return ErrNotAdmin
	}
	target, found, err := s.person(email)
	if err != nil {
		return err
	}
	if !found || target.Org != slug {
		return ErrNoMember
	}
	if target.Role == Admin {
		admins, err := s.admins(slug)
		if err != nil {
			return err
		}
		if admins <= 1 || target.Email == by.Email {
			return ErrLastAdmin
		}
	}
	if err := os.Remove(s.personFile(email)); err != nil {
		return err
	}
	s.publishRoster(slug)
	// Their computer is marked, and the organisation's server, which reaches the gateway,
	// removes it. A person who never had one has nothing marked.
	registry, err := s.Computers(slug)
	if err != nil {
		return err
	}
	return registry.Retire(email)
}
