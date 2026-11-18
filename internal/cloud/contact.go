package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// Contact is a note a visitor sent the team from the landing page.
type Contact struct {
	ID      string
	Sent    time.Time
	Email   string
	Company string
	Note    string
}

// Refusals a note to the team can meet.
var (
	ErrNeedNote      = errors.New("say what you would like to talk about")
	ErrNoteTooLong   = errors.New("a note is at most 2000 characters")
	ErrContactRepeat = errors.New("we have your notes from today, and will answer them")
	ErrContactsFull  = errors.New("we have had many notes today, so book a call or try again tomorrow")
)

// The most notes in a day: from one address, from one company, and from everybody. They keep a visitor, or a script that passes
// the bot check, from filling the team's mail.
const (
	contactsPerPerson  = 3
	contactsPerCompany = 10
	contactsPerDay     = 200
	noteMost           = 2000
	// ContactsShown is the most notes the operator's page lists, newest first.
	ContactsShown = 200
)

var insertContact = dbkit.Upsert{Table: "contacts", Columns: []string{"id", "sent", "email", "company", "note", "who", "domain"}, Conflict: []string{"id"}}

// SaveContact keeps a note from a visitor with a work address, within the day's limits, and is the note as it was kept.
func (s *Store) SaveContact(ctx context.Context, email, company, note string) (Contact, error) {
	contact, err := s.contactOf(email, company, note)
	if err != nil {
		return Contact{}, err
	}
	err = s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "contacts"); err != nil {
			return err
		}
		if err := s.contactAllowed(ctx, tx, contact.Email); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, insertContact.SQL(1), contact.ID, s.stamp(contact.Sent), contact.Email, contact.Company, contact.Note,
			withoutTag(contact.Email), DomainOf(contact.Email))
		return err
	})
	if err != nil {
		return Contact{}, err
	}
	return contact, nil
}

// contactOf is a note checked and made ready to keep: a work address, a company, and a note that is not empty or too long.
func (s *Store) contactOf(email, company, note string) (Contact, error) {
	note = strings.TrimSpace(note)
	switch {
	case note == "":
		return Contact{}, ErrNeedNote
	case utf8.RuneCountInString(note) > noteMost:
		return Contact{}, ErrNoteTooLong
	case strings.TrimSpace(company) == "":
		return Contact{}, ErrNeedCompany
	}
	address, err := NormalEmail(email)
	if err != nil {
		return Contact{}, err
	}
	if err := workEmail(address, s.set.AllowFreeMail, s.refused); err != nil {
		return Contact{}, err
	}
	name, err := OrgName(company)
	if err != nil {
		return Contact{}, err
	}
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		return Contact{}, err
	}
	return Contact{ID: hex.EncodeToString(id), Sent: s.now().UTC().Truncate(time.Second), Email: address, Company: name, Note: note}, nil
}

// contactAllowed refuses a note past the limits of the last twenty-four hours.
func (s *Store) contactAllowed(ctx context.Context, q dbkit.Querier, address string) error {
	since := s.stamp(s.now().Add(-24 * time.Hour))
	limits := []struct {
		query string
		arg   any
		most  int
		err   error
	}{
		{`SELECT count(*) FROM contacts WHERE sent >= ?`, nil, contactsPerDay, ErrContactsFull},
		{`SELECT count(*) FROM contacts WHERE who = ? AND sent >= ?`, withoutTag(address), contactsPerPerson, ErrContactRepeat},
		{`SELECT count(*) FROM contacts WHERE domain = ? AND sent >= ?`, DomainOf(address), contactsPerCompany, ErrContactRepeat},
	}
	for _, limit := range limits {
		args := []any{since}
		if limit.arg != nil {
			args = []any{limit.arg, since}
		}
		sent, err := count(ctx, q, limit.query, args...)
		if err != nil {
			return err
		}
		if sent >= limit.most {
			return limit.err
		}
	}
	return nil
}

// Contacts are the newest notes visitors sent, at most ContactsShown of them.
func (s *Store) Contacts(ctx context.Context) ([]Contact, error) {
	return collect(ctx, s.db, scanContact, `SELECT id, sent, email, company, note FROM contacts ORDER BY sent DESC, id LIMIT ?`, ContactsShown)
}

func scanContact(r rowScanner) (Contact, error) {
	var c Contact
	var sent dbkit.Time
	if err := r.Scan(&c.ID, &sent, &c.Email, &c.Company, &c.Note); err != nil {
		return Contact{}, err
	}
	c.Sent = sent.Time
	return c, nil
}
