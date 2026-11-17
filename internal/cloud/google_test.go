package cloud

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

// fakeGoogle is an identity provider that checks what Google checks: the PKCE verifier against the challenge it was given,
// and it signs the nonce it was given into the token.
type fakeGoogle struct {
	server  *httptest.Server
	key     *rsa.PrivateKey
	mu      sync.Mutex
	pending map[string]googleAsked
	// claims is what the next token says about who signed in, and nonce, when it is set, replaces the nonce it was given.
	claims map[string]any
	nonce  string
}

type googleAsked struct{ challenge, nonce string }

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	g := &fakeGoogle{key: key, pending: map[string]googleAsked{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": g.server.URL, "authorization_endpoint": g.server.URL + "/authorize", "token_endpoint": g.server.URL + "/token",
			"jwks_uri": g.server.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/authorize", g.authorize)
	mux.HandleFunc("/token", g.token)
	g.server = httptest.NewServer(mux)
	t.Cleanup(g.server.Close)
	return g
}

func (g *fakeGoogle) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" {
		http.Error(w, "no PKCE or nonce", http.StatusBadRequest)
		return
	}
	code := "code-" + q.Get("state")
	g.mu.Lock()
	g.pending[code] = googleAsked{challenge: q.Get("code_challenge"), nonce: q.Get("nonce")}
	g.mu.Unlock()
	http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
}

func (g *fakeGoogle) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	g.mu.Lock()
	was, known := g.pending[r.PostForm.Get("code")]
	delete(g.pending, r.PostForm.Get("code"))
	claims := map[string]any{"iss": g.server.URL, "aud": "client", "sub": "1", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": was.nonce}
	for k, v := range g.claims {
		claims[k] = v
	}
	if g.nonce != "" {
		claims["nonce"] = g.nonce
	}
	g.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !known || was.challenge != base64.RawURLEncoding.EncodeToString(sum[:]) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: g.key, KeyID: "k"}}, nil)
	payload, _ := json.Marshal(claims)
	signed, _ := signer.Sign(payload)
	compact, _ := signed.CompactSerialize()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "token_type": "Bearer", "id_token": compact})
}

// says sets who the next token is for.
func (g *fakeGoogle) says(email, hosted string, verified bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.claims = map[string]any{"email": email, "email_verified": verified}
	if hosted != "" {
		g.claims["hd"] = hosted
	}
}

// googleFront is gusto cloud with a client at the fake Google.
func googleFront(t *testing.T, set Settings) (*front, *fakeGoogle) {
	t.Helper()
	g := newFakeGoogle(t)
	google, err := NewGoogle(t.Context(), g.server.URL, "client", "secret", "http://cloud.test"+GoogleBack)
	if err != nil {
		t.Fatal(err)
	}
	return newFront(t, set, func(c *Config) { c.Google = google }), g
}

// throughGoogle is the round trip a browser makes: to gusto cloud, on to Google, and back with what Google answered.
func (b *browser) throughGoogle(g *fakeGoogle) (int, string, http.Header) {
	b.f.t.Helper()
	status, _, header := b.do("GET", "/signin/google", nil, nil)
	if status != http.StatusSeeOther || !strings.HasPrefix(header.Get("Location"), g.server.URL+"/authorize") {
		b.f.t.Fatalf("starting gave %d to %q", status, header.Get("Location"))
	}
	toGoogle := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	answered, err := toGoogle.Get(header.Get("Location"))
	if err != nil {
		b.f.t.Fatal(err)
	}
	answered.Body.Close()
	back, err := url.Parse(answered.Header.Get("Location"))
	if err != nil || back.Path != GoogleBack {
		b.f.t.Fatalf("Google sent the browser to %q", answered.Header.Get("Location"))
	}
	return b.do("GET", back.Path+"?"+back.RawQuery, nil, nil)
}

func (f *front) personOf(email string) Person {
	f.t.Helper()
	all, err := f.store.Memberships(f.t.Context(), email)
	if err != nil || len(all) != 1 {
		f.t.Fatalf("%s is in %d organizations: %v", email, len(all), err)
	}
	return all[0]
}

func TestSomebodyNewSignsUpWithGoogleAndTheirAddressNeedsNoMailToConfirm(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, g := googleFront(t, Settings{})
		g.says("ada@acme.com", "acme.com", true)
		b := f.browser()
		status, _, header := b.throughGoogle(g)
		if status != http.StatusSeeOther || header.Get("Location") != "/open" {
			t.Fatalf("%d to %q", status, header.Get("Location"))
		}
		if person := f.personOf("ada@acme.com"); person.Trial() {
			t.Error("the address Google confirmed is still on trial")
		}
		if mails := f.mails(); len(mails) != 0 {
			t.Errorf("a confirmation was mailed anyway: %q", mails)
		}
		if _, page := b.get("/signin"); !shown(t, page).Google {
			t.Error("the sign-in page does not offer Google")
		}
	})
}

func TestSomebodyWithAnAccountSignsInWithGoogle(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, g := googleFront(t, Settings{})
		f.signedUp("ada@acme.com")
		g.says("ada@acme.com", "acme.com", true)
		b := f.browser()
		if status, _, header := b.throughGoogle(g); status != http.StatusSeeOther || header.Get("Location") != "/app" {
			t.Fatalf("%d to %q", status, header.Get("Location"))
		}
		if status, _, header := b.do("GET", "/app", nil, nil); status != http.StatusSeeOther || header.Get("Location") == "/signin" {
			t.Errorf("the person is not signed in: %d to %q", status, header.Get("Location"))
		}
		if f.personOf("ada@acme.com").Trial() {
			t.Error("signing in with Google did not confirm the address")
		}
	})
}

// Where the address's domain cannot name the organization, the form asks for its name, and sending it signs the person up as
// the address Google confirmed, still with no mail to confirm it.
func TestAGoogleSignUpThatNeedsTheOrganizationsNameIsFinishedOnTheForm(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, g := googleFront(t, Settings{AllowFreeMail: true})
		g.says("ada@gmail.com", "", true)
		b := f.browser()
		status, page, _ := b.throughGoogle(g)
		if got := shown(t, page); status != http.StatusOK || got.Page != "signup" || !got.More || got.Email != "ada@gmail.com" || got.Ask == "" || got.Error != "" {
			t.Fatalf("%d: %+v", status, got)
		}
		if status, _, header := b.post("/signup", signup("ada@gmail.com", "Ada's team")); status != http.StatusSeeOther || header.Get("Location") != "/open" {
			t.Fatalf("%d to %q", status, header.Get("Location"))
		}
		if f.personOf("ada@gmail.com").Trial() {
			t.Error("the address Google confirmed is still on trial")
		}
		if mails := f.mails(); len(mails) != 0 {
			t.Errorf("a confirmation was mailed anyway: %q", mails)
		}

		other := f.browser()
		if status, _, _ := other.post("/signup", signup("bea@gmail.com", "Bea's team")); status != http.StatusSeeOther {
			t.Fatalf("%d", status)
		}
		if !f.personOf("bea@gmail.com").Trial() {
			t.Error("an address Google did not confirm in this browser was taken as confirmed")
		}
	})
}

func TestGoogleIsRefusedWhenItCannotSpeakForTheAddressOrTheAnswerIsNotThisSignIns(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, g := googleFront(t, Settings{})
		for name, says := range map[string]func(){
			"a company address on a personal Google account": func() { g.says("ada@acme.com", "", true) },
			"another Workspace domain":                       func() { g.says("ada@acme.com", "evil.com", true) },
			"an address Google did not verify":               func() { g.says("ada@acme.com", "acme.com", false) },
			"a token issued for another sign-in":             func() { g.says("ada@acme.com", "acme.com", true); g.nonce = "stolen" },
		} {
			says()
			b := f.browser()
			status, page, _ := b.throughGoogle(g)
			if got := shown(t, page); status != http.StatusBadRequest || got.Page != "signin" || got.Error == "" {
				t.Errorf("%s: %d %+v", name, status, got)
			}
			if found, _ := f.store.HasAccount(t.Context(), "ada@acme.com"); found {
				t.Fatalf("%s: an account was made", name)
			}
			g.nonce = ""
		}

		g.says("ada@acme.com", "acme.com", true)
		stranger := f.browser()
		if status, page := stranger.get(GoogleBack + "?code=code-x&state=x"); status != http.StatusBadRequest || shown(t, page).Error == "" {
			t.Errorf("an answer to a sign-in this browser did not start was taken: %d", status)
		}
	})
}

func TestWithoutAClientAtGoogleNothingOffersIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		b := f.browser()
		if status, _, header := b.do("GET", "/signin/google", nil, nil); status != http.StatusSeeOther || header.Get("Location") != "/signin" {
			t.Errorf("%d to %q", status, header.Get("Location"))
		}
		if _, page := b.get("/signup"); shown(t, page).Google {
			t.Error("the sign-up page offers Google")
		}
	})
}

func TestNewGoogleNeedsTheClientAndItsSecretTogether(t *testing.T) {
	if google, err := NewGoogle(t.Context(), "http://unused.test", "", "", ""); google != nil || err != nil {
		t.Errorf("no client: %v %v", google, err)
	}
	if _, err := NewGoogle(t.Context(), "http://unused.test", "client", "", ""); err == nil {
		t.Error("a client without its secret was taken")
	}
}

func TestGoogleSpeaksForGmailAndForItsOwnWorkspaceDomainOnly(t *testing.T) {
	for _, tc := range []struct {
		email, hosted string
		owns          bool
	}{
		{"ada@gmail.com", "", true},
		{"ada@googlemail.com", "", true},
		{"ada@acme.com", "acme.com", true},
		{"ada@acme.com", "ACME.com", true},
		{"ada@acme.com", "", false},
		{"ada@acme.com", "other.com", false},
		{"ada@sub.acme.com", "acme.com", false},
	} {
		if got := googleOwns(tc.email, tc.hosted); got != tc.owns {
			t.Errorf("%s in %q: %v", tc.email, tc.hosted, got)
		}
	}
}
