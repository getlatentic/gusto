package cloud

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var codeInMail = regexp.MustCompile(`Your sign-in code is (\d{6})`)

// askToSignIn asks for a sign-in for the address, and is the code and the link the mail held.
func askToSignIn(t *testing.T, f *front, email string) (code, token string) {
	t.Helper()
	f.browser().post("/signin", url.Values{"email": {email}})
	mails := f.mails()
	found := codeInMail.FindStringSubmatch(mails[len(mails)-1])
	if found == nil || !strings.Contains(mails[len(mails)-1], "Subject: Your gusto sign-in code is "+found[1]) {
		t.Fatalf("the mail holds no code:\n%s", mails[len(mails)-1])
	}
	link := f.lastLink()
	return found[1], link[strings.Index(link, "t=")+2:]
}

// A person signs in with the code they were mailed, typed as it was shown or with a space in it, and that spends the link too.
func TestTheCodeInTheMailSignsThePersonInOnceAndSpendsTheLink(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		f.browser().post("/signup", signup("ada@acme.com", "Acme"))
		f.mails()
		code, token := askToSignIn(t, f, "ada@acme.com")

		b := f.browser()
		typed := code[:3] + " " + code[3:]
		if status, _, header := b.post("/signin/code", url.Values{"email": {"Ada@Acme.com"}, "code": {typed}}); status != http.StatusSeeOther || header.Get("Location") != "/app" {
			t.Fatalf("the code answered %d %v", status, header)
		}
		if status, page := b.get("/org"); status != http.StatusOK || !strings.Contains(page, "Acme") {
			t.Errorf("the code did not sign the person in: %d", status)
		}
		if status, _, _ := f.browser().post("/signin/code", url.Values{"email": {"ada@acme.com"}, "code": {code}}); status != http.StatusUnprocessableEntity {
			t.Errorf("the code worked twice: %d", status)
		}
		if status, _, _ := f.browser().post("/signin/link", url.Values{"t": {token}}); status != http.StatusGone {
			t.Errorf("the link worked after the code: %d", status)
		}
	})
}

// Following the link spends the code mailed with it.
func TestTheLinkSpendsTheCodeMailedWithIt(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	f.browser().post("/signup", signup("ada@acme.com", "Acme"))
	f.mails()
	code, token := askToSignIn(t, f, "ada@acme.com")
	if status, _, header := f.browser().post("/signin/link", url.Values{"t": {token}}); status != http.StatusSeeOther || header.Get("Location") != "/app" {
		t.Fatalf("the link answered %d %v", status, header)
	}
	if status, _, _ := f.browser().post("/signin/code", url.Values{"email": {"ada@acme.com"}, "code": {code}}); status != http.StatusUnprocessableEntity {
		t.Errorf("the code worked after the link: %d", status)
	}
}

// A code works only for the address it was mailed to, an address with no account is answered as a wrong code, and after
// five tries an address takes no more codes for a while, even the right one.
func TestACodeWorksOnlyForItsAddressAndGuessesRunOut(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	f.browser().post("/signup", signup("ada@acme.com", "Acme"))
	f.browser().post("/signup", signup("bob@bob.com", "Bob"))
	f.mails()
	code, _ := askToSignIn(t, f, "ada@acme.com")

	_, wrong, _ := f.browser().post("/signin/code", url.Values{"email": {"bob@bob.com"}, "code": {code}})
	_, nobody, _ := f.browser().post("/signin/code", url.Values{"email": {"nobody@acme.com"}, "code": {code}})
	if !strings.Contains(wrong, "That code is wrong or has expired") ||
		strings.ReplaceAll(nobody, "nobody@acme.com", "bob@bob.com") != wrong {
		t.Errorf("another address's code, or one with no account, was answered differently:\n%s\n%s", wrong, nobody)
	}
	guess := "000000"
	if code == guess {
		guess = "111111"
	}
	for range codeTries {
		f.browser().post("/signin/code", url.Values{"email": {"ada@acme.com"}, "code": {guess}})
	}
	if status, _, _ := f.browser().post("/signin/code", url.Values{"email": {"ada@acme.com"}, "code": {code}}); status != http.StatusTooManyRequests {
		t.Errorf("the right code after %d wrong ones answered %d", codeTries, status)
	}
}
