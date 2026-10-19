package cloud

import (
	"context"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

var leadFields = []string{"slug", "name", "email", "company", "requested", "ready", "ended", "extended", "reason", "who", "domain"}

var insertLead = dbkit.Upsert{Table: "leads", Columns: leadFields, Conflict: []string{"slug"}}

const leadColumns = `slug, name, email, company, requested, ready, ended, extended, reason`

func (s *Store) leadArgs(l Lead) []any {
	return []any{l.Slug, l.Name, l.Email, l.Company, s.stamp(l.Requested), s.when(l.Ready), s.when(l.Ended), l.Extended, l.Reason,
		withoutTag(l.Email), DomainOf(strings.ToLower(l.Email))}
}

func scanLead(r rowScanner) (Lead, error) {
	var l Lead
	var requested dbkit.Time
	var ready, ended nullTime
	if err := r.Scan(&l.Slug, &l.Name, &l.Email, &l.Company, &requested, &ready, &ended, &l.Extended, &l.Reason); err != nil {
		return Lead{}, err
	}
	l.Requested, l.Ready, l.Ended = requested.Time, timeOf(ready), timeOf(ended)
	return l, nil
}

func (s *Store) putLead(ctx context.Context, tx dbkit.Tx, l Lead) error {
	_, err := tx.ExecContext(ctx, insertLead.SQL(1), s.leadArgs(l)...)
	return err
}

// LeadOf is the lead a demo made, found after the demo is gone.
func (s *Store) LeadOf(slug string) (Lead, bool) {
	if ValidSlug(slug) != nil {
		return Lead{}, false
	}
	lead, found, err := one(context.Background(), s.db, scanLead, `SELECT `+leadColumns+` FROM leads WHERE slug = ?`, slug)
	return lead, found && err == nil
}

// Leads are every visitor who started a demo, newest first.
func (s *Store) Leads() ([]Lead, error) {
	return collect(context.Background(), s.db, scanLead, `SELECT `+leadColumns+` FROM leads ORDER BY requested DESC, slug`)
}

// withinHabit refuses a visitor whose address, or whose company, has started as many demos as one may in the last
// twenty-four hours, which is counted from the leads that demos left, so it holds after a demo is deleted.
func (s *Store) withinHabit(ctx context.Context, q dbkit.Querier, address string, d DemoLimits) error {
	if d.PerPerson <= 0 && d.PerCompany <= 0 {
		return nil
	}
	since := s.stamp(s.now().Add(-24 * time.Hour))
	person, err := count(ctx, q, `SELECT count(*) FROM leads WHERE who = ? AND requested >= ?`, withoutTag(address), since)
	if err != nil {
		return err
	}
	company, err := count(ctx, q, `SELECT count(*) FROM leads WHERE domain = ? AND requested >= ?`, DomainOf(address), since)
	if err != nil {
		return err
	}
	if (d.PerPerson > 0 && person >= d.PerPerson) || (d.PerCompany > 0 && company >= d.PerCompany) {
		return ErrDemoRepeat
	}
	return nil
}

// withoutTag is an address without the part after a plus in its name, which is a way of making many addresses of one.
func withoutTag(address string) string {
	local, domain, _ := strings.Cut(strings.ToLower(address), "@")
	local, _, _ = strings.Cut(local, "+")
	return local + "@" + domain
}

// startedToday is how many demos were asked for since midnight UTC.
func (s *Store) startedToday(ctx context.Context, q dbkit.Querier) (int, error) {
	midnight := s.now().UTC().Truncate(24 * time.Hour)
	return count(ctx, q, `SELECT count(*) FROM leads WHERE requested >= ?`, s.stamp(midnight))
}
