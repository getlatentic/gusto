package cloud

import (
	"net/url"
	"strings"
	"testing"
)

func notifiedFront(t *testing.T, address string) *front {
	t.Helper()
	f := demoFront(t)
	f.server.Notify = address
	return f
}

func TestWhenSomeoneStartsADemoTheAddressThatFollowsUpIsMailed(t *testing.T) {
	f := notifiedFront(t, "tosin@getlatentic.com")
	startDemoAs(t, f, "Ada Lovelace", "ada@acme.com", "Acme Research")
	var found string
	for _, mail := range f.mails() {
		if strings.Contains(mail, "To: tosin@getlatentic.com") {
			found = mail
		}
	}
	for _, want := range []string{"Subject: New demo: Ada Lovelace, Acme Research", "Name: Ada Lovelace", "Email: ada@acme.com", "Company: Acme Research"} {
		if !strings.Contains(found, want) {
			t.Errorf("the notice lacks %q:\n%s", want, found)
		}
	}
}

func TestNothingIsMailedWhenNoAddressIsSet(t *testing.T) {
	f := notifiedFront(t, "")
	startDemoAs(t, f, "Ada Lovelace", "ada@acme.com", "Acme Research")
	if mails := f.mails(); len(mails) != 0 {
		t.Errorf("%d mails were sent:\n%v", len(mails), mails)
	}
}

func TestALineBreakInWhatAVisitorTypedDoesNotAddAHeaderToTheNotice(t *testing.T) {
	f := notifiedFront(t, "tosin@getlatentic.com")
	b := f.browser()
	b.post("/demo", url.Values{"name": {"Ada\r\nBcc: someone@evil.test"}, "email": {"ada@acme.com"}, "company": {"Acme"}})
	mails := f.mails()
	if len(mails) != 1 {
		t.Fatalf("%d mails", len(mails))
	}
	headers, _, _ := strings.Cut(mails[0], "\r\n\r\n")
	if strings.Contains(headers, "\r\nBcc:") {
		t.Errorf("a header was added:\n%s", headers)
	}
}
