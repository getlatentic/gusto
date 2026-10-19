package cloud

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// Purpose is what a link is for. A link made to verify an address does not sign
// anybody in, and the other way round.
type Purpose string

const (
	Verify Purpose = "verify"
	SignIn Purpose = "signin"
	// Invite is a link an admin sent to a person, to join their organization.
	Invite Purpose = "invite"
)

// ErrLink is every reason a link does not work, said once: one that was used,
// one that ran out, and one that never was.
var ErrLink = errors.New("that link has been used or has expired, so ask for another")

type link struct {
	Email   string    `json:"email"`
	Purpose Purpose   `json:"purpose"`
	Expires time.Time `json:"expires"`
	// An invitation is for an organization, in a role, from a person.
	Org  string `json:"org,omitempty"`
	Role Role   `json:"role,omitempty"`
	By   string `json:"by,omitempty"`
}

// Lives are how long a link works.
const (
	VerifyLife = 3 * 24 * time.Hour
	SignInLife = 20 * time.Minute
)

const longestSecret = 128

// digest is how a link's secret is kept: only the hash is in the database, so the database cannot be
// used to sign in.
func digest(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

var linkFields = []string{"hash", "email", "purpose", "expires", "org", "role", "invited_by"}

var insertLink = dbkit.Upsert{Table: "links", Columns: linkFields, Conflict: []string{"hash"}}

func (s *Store) linkArgs(hash string, l link) []any {
	return []any{hash, l.Email, string(l.Purpose), s.stamp(l.Expires), l.Org, string(l.Role), l.By}
}

func scanLink(r rowScanner) (link, error) {
	var l link
	var purpose, role string
	var expires dbkit.Time
	if err := r.Scan(&l.Email, &purpose, &expires, &l.Org, &role, &l.By); err != nil {
		return link{}, err
	}
	l.Purpose, l.Role, l.Expires = Purpose(purpose), Role(role), expires.Time
	return l, nil
}

// MakeLink is the secret of a single-use link for an address.
func (s *Store) MakeLink(email string, purpose Purpose, life time.Duration) (string, error) {
	ctx := context.Background()
	var secret string
	err := s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
		secret, err = s.putLink(ctx, tx, link{Email: email, Purpose: purpose, Expires: s.now().Add(life)})
		return err
	})
	return secret, err
}

func (s *Store) putLink(ctx context.Context, tx dbkit.Tx, made link) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	_, err := tx.ExecContext(ctx, insertLink.SQL(1), s.linkArgs(digest(secret), made)...)
	return secret, err
}

// UseLink is the address a link was made for, and the link is spent. It is deleted in
// the step that reads it, and a row has one deleter, so two requests with the same link
// cannot both get in, even on two front doors.
func (s *Store) UseLink(secret string, purpose Purpose) (string, error) {
	used, err := s.useLink(secret, purpose)
	return used.Email, err
}

func (s *Store) useLink(secret string, purpose Purpose) (link, error) {
	if secret == "" || len(secret) > longestSecret {
		return link{}, ErrLink
	}
	ctx := context.Background()
	var made link
	var found bool
	err := s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
		made, found, err = s.spend(ctx, tx, digest(secret))
		return err
	})
	if err != nil {
		return link{}, err
	}
	if !found || made.Purpose != purpose || !s.now().Before(made.Expires) {
		return link{}, ErrLink
	}
	return made, nil
}

// spend deletes a link and returns what it was.
func (s *Store) spend(ctx context.Context, tx dbkit.Tx, hash string) (link, bool, error) {
	return one(ctx, tx, scanLink, `DELETE FROM links WHERE hash = ? RETURNING email, purpose, expires, org, role, invited_by`, hash)
}

// Sweep removes links that have run out, and returns how many. Invitations and rate counts that have
// run out go with them.
func (s *Store) Sweep() (int, error) {
	ctx, now := context.Background(), s.stamp(s.now())
	done, err := s.db.ExecContext(ctx, `DELETE FROM links WHERE expires <= ?`, now)
	if err != nil {
		return 0, err
	}
	for _, table := range []struct{ name, column string }{{"invitations", "expires"}, {"rate_events", "until"}} {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM `+table.name+` WHERE `+table.column+` <= ?`, now); err != nil {
			return 0, err
		}
	}
	removed, err := done.RowsAffected()
	return int(removed), err
}

// peekInvitation is what an invitation link is for, without spending it, so the page
// that asks a person to join can say what they are joining. A link that is not an
// invitation, has run out or never was is not found.
func (s *Store) peekInvitation(secret string) (link, bool) {
	if secret == "" || len(secret) > longestSecret {
		return link{}, false
	}
	made, found, err := one(context.Background(), s.db, scanLink,
		`SELECT email, purpose, expires, org, role, invited_by FROM links WHERE hash = ?`, digest(secret))
	if err != nil || !found || made.Purpose != Invite || !s.now().Before(made.Expires) {
		return link{}, false
	}
	return made, true
}
