package cloud

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Purpose is what a link is for. A link made to verify an address does not sign
// anybody in, and the other way round.
type Purpose string

const (
	Verify Purpose = "verify"
	SignIn Purpose = "signin"
	// Invite is a link an admin sent to a person, to join their organisation.
	Invite Purpose = "invite"
)

// ErrLink is every reason a link does not work, said once: one that was used,
// one that ran out, and one that never was.
var ErrLink = errors.New("that link has been used or has expired, so ask for another")

type link struct {
	Email   string    `json:"email"`
	Purpose Purpose   `json:"purpose"`
	Expires time.Time `json:"expires"`
	// An invitation is for an organisation, in a role, from a person.
	Org  string `json:"org,omitempty"`
	Role Role   `json:"role,omitempty"`
	By   string `json:"by,omitempty"`
}

// Lives are how long a link works.
const (
	VerifyLife = 3 * 24 * time.Hour
	SignInLife = 20 * time.Minute
)

func (s *Store) linkFile(secret, suffix string) string {
	sum := sha256.Sum256([]byte(secret))
	return filepath.Join(s.root, "tokens", hex.EncodeToString(sum[:])+suffix)
}

// MakeLink is the secret of a single-use link for an address. Only the hash of
// the secret is kept, so the files here cannot be used to sign in.
func (s *Store) MakeLink(email string, purpose Purpose, life time.Duration) (string, error) {
	return s.makeLink(link{Email: email, Purpose: purpose, Expires: s.now().Add(life)})
}

func (s *Store) makeLink(made link) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	return secret, writeFile(s.linkFile(secret, ".json"), made, 0o600)
}

// UseLink is the address a link was made for, and the link is spent. It is
// renamed out of the way before it is read, and a rename has one winner, so two
// requests with the same link cannot both get in.
func (s *Store) UseLink(secret string, purpose Purpose) (string, error) {
	used, err := s.useLink(secret, purpose)
	return used.Email, err
}

func (s *Store) useLink(secret string, purpose Purpose) (link, error) {
	if secret == "" || len(secret) > 128 {
		return link{}, ErrLink
	}
	spent := s.linkFile(secret, ".spent")
	if err := os.Rename(s.linkFile(secret, ".json"), spent); err != nil {
		return link{}, ErrLink
	}
	defer os.Remove(spent)
	var made link
	if err := readFile(spent, &made); err != nil || made.Purpose != purpose || !s.now().Before(made.Expires) {
		return link{}, ErrLink
	}
	return made, nil
}

// Sweep removes links that have run out, and returns how many.
func (s *Store) Sweep() (int, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "tokens"))
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		path := filepath.Join(s.root, "tokens", entry.Name())
		var made link
		if readFile(path, &made) == nil && s.now().Before(made.Expires) {
			continue
		}
		if os.Remove(path) == nil {
			removed++
		}
	}
	return removed, nil
}

// peekInvitation is what an invitation link is for, without spending it, so the page
// that asks a person to join can say what they are joining. A link that is not an
// invitation, has run out or never was is not found.
func (s *Store) peekInvitation(secret string) (link, bool) {
	if secret == "" || len(secret) > 128 {
		return link{}, false
	}
	var made link
	if err := readFile(s.linkFile(secret, ".json"), &made); err != nil || made.Purpose != Invite || !s.now().Before(made.Expires) {
		return link{}, false
	}
	return made, true
}
