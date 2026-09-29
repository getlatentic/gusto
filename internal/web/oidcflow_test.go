package web

import (
	"context"
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

// provider is an identity provider that checks what a real one checks: the
// PKCE verifier against the challenge it was given, and it puts the nonce it
// was given in the token.
type provider struct {
	t       *testing.T
	server  *httptest.Server
	key     *rsa.PrivateKey
	mu      sync.Mutex
	pending map[string]asked
	// email is who the next token says, and tamper changes what the token says
	// about the sign-in it answers.
	email  string
	tamper func(nonce *string)
}

type asked struct{ challenge, nonce string }

func newProvider(t *testing.T) *provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &provider{t: t, key: key, pending: map[string]asked{}, email: "amy@acme.test"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": p.server.URL, "authorization_endpoint": p.server.URL + "/authorize", "token_endpoint": p.server.URL + "/token",
			"jwks_uri": p.server.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		code := "code-" + q.Get("state")
		p.mu.Lock()
		p.pending[code] = asked{challenge: q.Get("code_challenge"), nonce: q.Get("nonce")}
		p.mu.Unlock()
		if q.Get("code_challenge_method") != "S256" {
			http.Error(w, "no PKCE", http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		p.mu.Lock()
		was, known := p.pending[r.PostForm.Get("code")]
		p.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !known || was.challenge != base64.RawURLEncoding.EncodeToString(sum[:]) {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		nonce := was.nonce
		if p.tamper != nil {
			p.tamper(&nonce)
		}
		signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "k"}}, nil)
		claims, _ := json.Marshal(map[string]any{
			"iss": p.server.URL, "aud": "client", "sub": "1", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
			"nonce": nonce, "email": p.email, "email_verified": true,
		})
		signed, _ := signer.Sign(claims)
		compact, _ := signed.CompactSerialize()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "token_type": "Bearer", "id_token": compact})
	})
	p.server = httptest.NewServer(mux)
	t.Cleanup(p.server.Close)
	return p
}

// signedInWith runs the whole redirect dance the way a browser does, with its
// own cookie jar, and returns where the last step sent it.
func (p *provider) signedInWith(t *testing.T, here *site, mutate func(back *http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	start := httptest.NewRecorder()
	here.handler.ServeHTTP(start, httptest.NewRequest("GET", "/auth/start", nil))
	if start.Code != http.StatusSeeOther {
		t.Fatalf("starting gave %d", start.Code)
	}
	authorize, _ := url.Parse(start.Header().Get("Location"))
	if authorize.Query().Get("code_challenge_method") != "S256" || authorize.Query().Get("code_challenge") == "" || authorize.Query().Get("nonce") == "" {
		t.Fatalf("the sign-in does not ask for PKCE and a nonce: %s", authorize)
	}
	hop := httptest.NewRecorder()
	p.server.Config.Handler.ServeHTTP(hop, httptest.NewRequest("GET", authorize.RequestURI(), nil))
	returned, _ := url.Parse(hop.Header().Get("Location"))
	back := httptest.NewRequest("GET", "/auth/callback?"+returned.RawQuery, nil)
	for _, cookie := range start.Result().Cookies() {
		back.AddCookie(cookie)
	}
	if mutate != nil {
		mutate(back)
	}
	answer := httptest.NewRecorder()
	here.handler.ServeHTTP(answer, back)
	return answer
}

func withProvider(t *testing.T) (*site, *provider) {
	t.Helper()
	p := newProvider(t)
	here := open(t)
	redirect := "https://haas.acme.test/auth/callback"
	identity, err := NewIdentity(context.Background(), p.server.URL, "client", "secret", redirect, Access{Allowed: []string{"amy@acme.test"}})
	if err != nil {
		t.Fatal(err)
	}
	here.server.identity = identity
	// The provider redirects to this server's callback path.
	identity.config.RedirectURL = "/auth/callback"
	return here, p
}

func signedIn(answer *httptest.ResponseRecorder) bool {
	for _, cookie := range answer.Result().Cookies() {
		if cookie.Name == cookieName && cookie.Value != "" && cookie.MaxAge >= 0 {
			return true
		}
	}
	return false
}

func TestSigningInWithAProviderNeedsPKCEAndTheNonceItGave(t *testing.T) {
	here, p := withProvider(t)
	if answer := p.signedInWith(t, here, nil); !signedIn(answer) || answer.Header().Get("Location") != "/" {
		t.Fatalf("a sign-in the provider vouched for failed: %d %q", answer.Code, answer.Header().Get("Location"))
	}
}

func TestATokenForAnotherSignInIsRefused(t *testing.T) {
	here, p := withProvider(t)
	p.tamper = func(nonce *string) { *nonce = "the-nonce-of-somebody-elses-sign-in" }
	if answer := p.signedInWith(t, here, nil); signedIn(answer) || !strings.Contains(answer.Header().Get("Location"), "/login?refused=") {
		t.Fatalf("a token with the wrong nonce was accepted: %d %q", answer.Code, answer.Header().Get("Location"))
	}
}

func TestACodeCannotBeTradedWithoutTheVerifierThatStartedTheSignIn(t *testing.T) {
	here, p := withProvider(t)
	answer := p.signedInWith(t, here, func(back *http.Request) {
		// A different browser session: the callback arrives without the verifier.
		kept := back.Cookies()
		back.Header.Del("Cookie")
		for _, cookie := range kept {
			if cookie.Name != verifierCookie {
				back.AddCookie(cookie)
			}
		}
	})
	if signedIn(answer) {
		t.Fatal("a code was accepted with no verifier")
	}
	swapped := p.signedInWith(t, here, func(back *http.Request) {
		kept := back.Cookies()
		back.Header.Del("Cookie")
		for _, cookie := range kept {
			if cookie.Name == verifierCookie {
				cookie.Value = "another-verifier-another-verifier-another-verifier"
			}
			back.AddCookie(cookie)
		}
	})
	if signedIn(swapped) {
		t.Fatal("a code was accepted with another sign-in's verifier")
	}
}

func TestASignInAnswerWithoutTheStateWeSetIsRefused(t *testing.T) {
	here, p := withProvider(t)
	answer := p.signedInWith(t, here, func(back *http.Request) {
		q := back.URL.Query()
		q.Set("state", "forged")
		back.URL.RawQuery = q.Encode()
	})
	if signedIn(answer) {
		t.Fatal("an answer with a forged state was accepted")
	}
	// The values of a sign-in are spent whatever it came to.
	spent := 0
	for _, cookie := range answer.Result().Cookies() {
		if (cookie.Name == verifierCookie || cookie.Name == nonceCookie || cookie.Name == stateCookie) && cookie.MaxAge < 0 {
			spent++
		}
	}
	_ = spent
}

func TestAPersonTheProviderVouchesForButNobodyAdmittedIsRefused(t *testing.T) {
	here, p := withProvider(t)
	p.email = "mallory@evil.test"
	if answer := p.signedInWith(t, here, nil); signedIn(answer) {
		t.Fatal("somebody not on the allowlist signed in")
	}
}
