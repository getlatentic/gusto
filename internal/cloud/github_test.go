package cloud

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// fakeGitHub answers the two calls a sign-in makes, as GitHub does: it gives a token only for the code it handed out, to the
// client's secret and the verifier of the challenge the code was asked with, and lists the person's addresses for that token.
type fakeGitHub struct {
	*httptest.Server
	emails []githubEmail
	mu     sync.Mutex
	codes  map[string]string // code to its challenge
}

func newFakeGitHub(t *testing.T, emails ...githubEmail) *fakeGitHub {
	g := &fakeGitHub{emails: emails, codes: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		challenge, found := g.codes[r.PostFormValue("code")]
		delete(g.codes, r.PostFormValue("code"))
		g.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostFormValue("code_verifier")))
		w.Header().Set("Content-Type", "application/json")
		if !found || r.PostFormValue("client_id") != "Ov23li" || r.PostFormValue("client_secret") != "shh" ||
			base64.RawURLEncoding.EncodeToString(sum[:]) != challenge || r.PostFormValue("redirect_uri") != "http://cloud.test/signin/github/callback" {
			json.NewEncoder(w).Encode(map[string]string{"error": "bad_verification_code"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "gho_person"})
	})
	mux.HandleFunc("GET /user/emails", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer gho_person" {
			http.Error(w, "bad credentials", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(g.emails)
	})
	g.Server = httptest.NewServer(mux)
	t.Cleanup(g.Close)
	return g
}

// approve is the person allowing gusto at GitHub: GitHub hands back a code for the challenge, with the state it was given.
func (g *fakeGitHub) approve(t *testing.T, authorize string) url.Values {
	t.Helper()
	asked, err := url.Parse(authorize)
	if err != nil || !strings.HasPrefix(authorize, g.URL+"/login/oauth/authorize?") {
		t.Fatalf("the person was not sent to GitHub: %q", authorize)
	}
	query := asked.Query()
	if query.Get("client_id") != "Ov23li" || query.Get("scope") != "user:email" || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("GitHub was asked %v", query)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	code := "code-" + query.Get("state")[:6]
	g.codes[code] = query.Get("code_challenge")
	return url.Values{"code": {code}, "state": {query.Get("state")}}
}

func gitHubFront(t *testing.T, g *fakeGitHub) *front {
	return newFront(t, Settings{}, func(c *Config) {
		c.GitHub = &GitHub{ID: "Ov23li", Secret: "shh", HTTP: g.Client(), Site: g.URL, API: g.URL}
	})
}

// signInWithGitHub is a person pressing Continue with GitHub, allowing it there, and coming back.
func (b *browser) signInWithGitHub(t *testing.T, g *fakeGitHub) (int, string, http.Header) {
	t.Helper()
	status, _, header := b.do("GET", "/signin/github", nil, nil)
	if status != http.StatusSeeOther {
		t.Fatalf("Continue with GitHub answered %d", status)
	}
	return b.do("GET", "/signin/github/callback?"+g.approve(t, header.Get("Location")).Encode(), nil, nil)
}

// A person whose account is under an address GitHub verified is signed in by GitHub, whichever of their addresses it is, and an
// address GitHub has not verified signs nobody in.
func TestGitHubSignsInThePersonWhoseAccountIsAnAddressItVerified(t *testing.T) {
	g := newFakeGitHub(t,
		githubEmail{Email: "ada@gmail.com", Primary: true, Verified: true},
		githubEmail{Email: "bob@initech.com", Verified: false},
		githubEmail{Email: "Ada@Acme.com", Verified: true})
	f := gitHubFront(t, g)
	f.browser().post("/signup", signup("bob@initech.com", "Initech"))
	f.browser().post("/signup", signup("ada@acme.com", "Acme Labs"))
	f.mails()

	if _, page := f.browser().get("/signin"); !strings.Contains(page, `href="/signin/github"`) {
		t.Fatalf("the sign-in page offers no GitHub:\n%s", page)
	}
	ada := f.browser()
	if status, _, header := ada.signInWithGitHub(t, g); status != http.StatusSeeOther || header.Get("Location") != "/" {
		t.Fatalf("coming back from GitHub answered %d %v", status, header)
	}
	if status, page := ada.get("/org"); status != http.StatusOK || !strings.Contains(page, "Acme Labs") {
		t.Errorf("the person is not in their own organization: %d", status)
	}
}

// Coming back from GitHub with no sign-in started in this browser, with another browser's state, or after saying no there,
// signs nobody in.
func TestASignInWithGitHubThisBrowserDidNotStartSignsNobodyIn(t *testing.T) {
	g := newFakeGitHub(t, githubEmail{Email: "ada@acme.com", Primary: true, Verified: true})
	f := gitHubFront(t, g)
	f.browser().post("/signup", signup("ada@acme.com", "Acme"))
	f.mails()

	started := f.browser()
	_, _, header := started.do("GET", "/signin/github", nil, nil)
	answer := g.approve(t, header.Get("Location"))
	stranger := f.browser()
	if status, _, _ := stranger.do("GET", "/signin/github/callback?"+answer.Encode(), nil, nil); status != http.StatusBadRequest {
		t.Errorf("a browser that started nothing came back with %d", status)
	}
	forged := url.Values{"code": answer["code"], "state": {"not-the-state"}}
	if status, _, _ := started.do("GET", "/signin/github/callback?"+forged.Encode(), nil, nil); status != http.StatusBadRequest {
		t.Errorf("another state came back with %d", status)
	}
	for _, b := range []*browser{started, stranger} {
		if status, _ := b.get("/org"); status != http.StatusSeeOther {
			t.Errorf("somebody was signed in: %d", status)
		}
	}

	declined := f.browser()
	_, _, header = declined.do("GET", "/signin/github", nil, nil)
	state := g.approve(t, header.Get("Location")).Get("state")
	status, page, _ := declined.do("GET", "/signin/github/callback?"+url.Values{"error": {"access_denied"}, "state": {state}}.Encode(), nil, nil)
	if status != http.StatusBadRequest || !strings.Contains(page, "GitHub did not sign you in") {
		t.Errorf("saying no at GitHub answered %d:\n%s", status, page)
	}
}

// A person with no account is asked to make an organization with a work address GitHub verified, and that address needs no
// mail to confirm it; an address they type instead is mailed as any other.
func TestAPersonNewToGustoSignsUpWithTheWorkAddressGitHubVerified(t *testing.T) {
	g := newFakeGitHub(t,
		githubEmail{Email: "grace@gmail.com", Primary: true, Verified: true},
		githubEmail{Email: "grace@globex.com", Verified: true})
	f := gitHubFront(t, g)
	grace := f.browser()
	status, page, _ := grace.signInWithGitHub(t, g)
	if status != http.StatusOK || !strings.Contains(page, `value="grace@globex.com"`) || !strings.Contains(page, "GitHub verified grace@globex.com") {
		t.Fatalf("a new person was not asked to sign up with their work address: %d\n%s", status, page)
	}
	if status, _, _ := grace.post("/signup", signup("grace@globex.com", "Globex")); status != http.StatusSeeOther {
		t.Fatalf("signing up answered %d", status)
	}
	if person, _, err := f.store.only("grace@globex.com"); err != nil || person.Trial() {
		t.Errorf("the address GitHub verified is still to be confirmed: %+v %v", person, err)
	}
	if mails := f.mails(); len(mails) != 0 {
		t.Errorf("a confirmation was mailed for an address GitHub verified: %v", mails)
	}

	typed := f.browser()
	typed.signInWithGitHub(t, g)
	typed.post("/signup", signup("grace@initech.com", "Initech"))
	if person, _, err := f.store.only("grace@initech.com"); err != nil || !person.Trial() {
		t.Errorf("an address GitHub did not verify was taken as verified: %+v %v", person, err)
	}
	if mails := f.mails(); len(mails) != 1 || !strings.Contains(mails[0], "grace@initech.com") {
		t.Errorf("the typed address was not mailed to confirm it: %v", mails)
	}
}

// A front door with no GitHub client offers no sign-in with GitHub.
func TestAFrontDoorWithoutGitHubsClientOffersNoSignInWithIt(t *testing.T) {
	f := newFront(t, Settings{}, nil)
	if status, _ := f.browser().get("/signin/github"); status != http.StatusNotFound {
		t.Errorf("starting a sign-in with GitHub answered %d", status)
	}
	if _, page := f.browser().get("/signin"); strings.Contains(page, "GitHub") {
		t.Errorf("the sign-in page offers GitHub:\n%s", page)
	}
}
