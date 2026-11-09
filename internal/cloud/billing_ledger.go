package cloud

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// eventSeen says a processor's notice was applied before; markEvent remembers that it was.
func (s *Store) eventSeen(ctx context.Context, provider, id string) (bool, error) {
	n, err := count(ctx, s.db, `SELECT count(*) FROM billing_events WHERE provider = ? AND id = ?`, provider, id)
	return n > 0, err
}

var putEvent = dbkit.Upsert{Table: "billing_events", Columns: []string{"provider", "id", "kind", "received"}, Conflict: []string{"provider", "id"}}

func (s *Store) markEvent(ctx context.Context, provider, id, kind string) error {
	_, err := s.db.ExecContext(ctx, putEvent.SQL(1), provider, id, kind, s.stamp(s.now().UTC()))
	return err
}

// newSecret is a random string a program holds; digest is what is kept of it.
func newSecret(prefix string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

var putToken = dbkit.Upsert{Table: "billing_agent_tokens", Columns: []string{"org", "digest", "created"}, Conflict: []string{"org"}, Update: []string{"digest", "created"}}

// IssueAgentToken makes the bearer token an organization's agents ask about billing with. It replaces the old
// one, and is returned once: only its digest is kept.
func (s *Store) IssueAgentToken(org string) (string, error) {
	secret, err := newSecret("gbt_")
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(context.Background(), putToken.SQL(1), org, digest(secret), s.stamp(s.now().UTC()))
	return secret, err
}

// OrgOfAgentToken is the organization a bearer token was issued to.
func (s *Store) OrgOfAgentToken(secret string) (string, bool, error) {
	return one(context.Background(), s.db, func(r rowScanner) (string, error) {
		var org string
		return org, r.Scan(&org)
	}, `SELECT org FROM billing_agent_tokens WHERE digest = ?`, digest(secret))
}
