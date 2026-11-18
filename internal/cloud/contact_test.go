package cloud

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func noteForm(email, company, note string) url.Values {
	return url.Values{"email": {email}, "company": {company}, "note": {note}}
}

// A visitor's note is kept, mailed to the address that follows demos up with what they wrote, and the visitor is sent to the
// landing page, which thanks them.
func TestANoteFromTheLandingPageReachesTheTeam(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := notifiedFront(t, "tosin@getlatentic.com")
		b := f.browser()
		status, _, header := b.post("/contact", noteForm("Maria@FutureAcmeBank.com", "Future Acme Bank", "We have 40 analysts.\nCan agents read our filings?"))
		if status != http.StatusSeeOther || header.Get("Location") != contactSent {
			t.Fatalf("the note was answered %d %v", status, header)
		}
		if got := shownAt(t, b, "/?contact=sent"); got.Contact == nil || !got.Contact.Sent {
			t.Errorf("the visitor is not thanked: %+v", got.Contact)
		}
		kept, err := f.store.Contacts(t.Context())
		if err != nil || len(kept) != 1 || kept[0].Email != "maria@futureacmebank.com" || kept[0].Company != "Future Acme Bank" ||
			!strings.Contains(kept[0].Note, "Can agents read our filings?") {
			t.Fatalf("the note was not kept: %+v %v", kept, err)
		}
		mails := f.mails()
		if len(mails) != 1 {
			t.Fatalf("%d mails", len(mails))
		}
		for _, want := range []string{"To: tosin@getlatentic.com", "Subject: Note from Future Acme Bank: maria@futureacmebank.com",
			"Email: maria@futureacmebank.com", "We have 40 analysts.", "Can agents read our filings?"} {
			if !strings.Contains(mails[0], want) {
				t.Errorf("the mail lacks %q:\n%s", want, mails[0])
			}
		}
	})
}

// A note that is refused is answered with the landing page, why, and what was typed, and nothing is kept or mailed.
func TestARefusedNoteKeepsWhatWasTyped(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := notifiedFront(t, "tosin@getlatentic.com")
		for _, refused := range []struct {
			form   url.Values
			status int
			says   string
		}{
			{noteForm("maria@futureacmebank.com", "Future Acme Bank", "  "), http.StatusUnprocessableEntity, "Say what you would like to talk about"},
			{noteForm("maria@futureacmebank.com", "Future Acme Bank", strings.Repeat("é", noteMost+1)), http.StatusUnprocessableEntity, "at most 2000"},
			{noteForm("maria@futureacmebank.com", "", "Hello"), http.StatusUnprocessableEntity, "Say which company"},
			{noteForm("maria", "Future Acme Bank", "Hello"), http.StatusUnprocessableEntity, "not an email address"},
			{noteForm("maria@mailinator.com", "Future Acme Bank", "Hello"), http.StatusUnprocessableEntity, "expire"},
		} {
			status, page, _ := f.browser().post("/contact", refused.form)
			got := shown(t, page)
			if status != refused.status || got.Page != "landing" || got.Contact == nil || !strings.Contains(got.Contact.Error, refused.says) ||
				got.Contact.Email != refused.form.Get("email") || got.Contact.Note != refused.form.Get("note") {
				t.Errorf("%v was answered %d %+v", refused.form, status, got.Contact)
			}
		}
		if kept, _ := f.store.Contacts(t.Context()); len(kept) != 0 {
			t.Errorf("a refused note was kept: %+v", kept)
		}
		if mails := f.mails(); len(mails) != 0 {
			t.Errorf("a refused note was mailed: %v", mails)
		}
	})
}

// One address, with or without a tag, sends three notes in a day, one company ten, and everybody two hundred. A day later they
// may send again.
func TestNotesAreLimitedByAddressCompanyAndDay(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		now := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
		f.store.now = func() time.Time { return now }
		save := func(email string) error {
			_, err := f.store.SaveContact(t.Context(), email, "Future Acme Bank", "Hello")
			return err
		}
		for _, email := range []string{"maria@futureacmebank.com", "maria+1@futureacmebank.com", "Maria@futureacmebank.com"} {
			if err := save(email); err != nil {
				t.Fatal(err)
			}
		}
		if err := save("maria+2@futureacmebank.com"); !errors.Is(err, ErrContactRepeat) {
			t.Errorf("a fourth note from one address: %v", err)
		}
		for i := range contactsPerCompany - contactsPerPerson {
			if err := save(fmt.Sprintf("analyst%d@futureacmebank.com", i)); err != nil {
				t.Fatal(err)
			}
		}
		if err := save("ceo@futureacmebank.com"); !errors.Is(err, ErrContactRepeat) {
			t.Errorf("an eleventh note from one company: %v", err)
		}
		for i := range contactsPerDay - contactsPerCompany {
			if err := save(fmt.Sprintf("someone@company%d.com", i)); err != nil {
				t.Fatal(err)
			}
		}
		if err := save("someone@onemore.com"); !errors.Is(err, ErrContactsFull) {
			t.Errorf("a note past the day's: %v", err)
		}

		now = now.Add(24*time.Hour + time.Second)
		if err := save("maria@futureacmebank.com"); err != nil {
			t.Errorf("a note a day later: %v", err)
		}
		if kept, _ := f.store.Contacts(t.Context()); len(kept) != ContactsShown || kept[0].Email != "maria@futureacmebank.com" {
			t.Errorf("the newest %d notes, newest first, are listed: %d, first %+v", ContactsShown, len(kept), kept[0])
		}
	})
}

// Where there is a bot check, the landing page carries it and may load its script from Cloudflare, and a note is kept only for a
// visitor Cloudflare accepts. Where there is none, the landing page loads nothing from Cloudflare.
func TestANoteIsKeptOnlyForAVisitorCloudflareAccepts(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := checkedFront(t, newCloudflare(t))
		status, page, header := f.browser().do("GET", "/", nil, nil)
		if got := shown(t, page); status != http.StatusOK || got.TurnstileSite != "the-site-key" || got.Contact == nil {
			t.Errorf("the landing page has no bot check for the note: %d %+v", status, got.Contact)
		}
		if csp := header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self' https://challenges.cloudflare.com") {
			t.Errorf("the landing page may not load the check: %s", csp)
		}
		form := noteForm("maria@futureacmebank.com", "Future Acme Bank", "Hello")
		form.Set("cf-turnstile-response", "a-robot")
		if status, page, _ := f.browser().post("/contact", form); status != http.StatusForbidden || !strings.Contains(shown(t, page).Contact.Error, "robot") {
			t.Errorf("a robot's note was answered %d", status)
		}
		form.Set("cf-turnstile-response", "a-human")
		if status, _, _ := f.browser().post("/contact", form); status != http.StatusSeeOther {
			t.Errorf("a person's note was answered %d", status)
		}
		if kept, _ := f.store.Contacts(t.Context()); len(kept) != 1 {
			t.Errorf("%d notes were kept", len(kept))
		}

		_, plain, plainHeader := demoFront(t).browser().do("GET", "/", nil, nil)
		if shown(t, plain).TurnstileSite != "" || strings.Contains(plainHeader.Get("Content-Security-Policy"), "cloudflare") {
			t.Error("a landing page with no bot check loads a script from Cloudflare")
		}
	})
}

func TestAHostWithNoLandingPageTakesNoNotes(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		if status, _, _ := f.browser().post("/contact", noteForm("maria@futureacmebank.com", "Future Acme Bank", "Hello")); status != http.StatusNotFound {
			t.Errorf("%d", status)
		}
	})
}
