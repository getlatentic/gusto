package cloud

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// GoogleIssuer is Google's sign-in, whose discovery document names its endpoints and its signing keys.
const GoogleIssuer = "https://accounts.google.com"

// GoogleBack is where Google sends a person back to, which the OAuth client at Google must list as a redirect address.
const GoogleBack = "/signin/google/back"

// googleWait bounds each call to Google, so a slow answer cannot hold a request open.
const googleWait = 15 * time.Second

// Google is signing in with a Google account. A nil Google is none, and the pages offer only email.
type Google struct {
	config   oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// NewGoogle reads the issuer's discovery document, so a wrong issuer fails when gusto cloud starts and not at somebody's first
// sign-in. redirect is gusto cloud's own address for GoogleBack. With neither a client id nor a secret there is no Google.
func NewGoogle(ctx context.Context, issuer, clientID, secret, redirect string) (*Google, error) {
	if clientID == "" && secret == "" {
		return nil, nil
	}
	if clientID == "" || secret == "" {
		return nil, errors.New("Google sign-in needs both its client id and its secret")
	}
	ctx, cancel := context.WithTimeout(ctx, googleWait)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("could not reach %s: %w", issuer, err)
	}
	return &Google{
		config: oauth2.Config{ClientID: clientID, ClientSecret: secret, RedirectURL: redirect,
			Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "email", "profile"}},
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
	}, nil
}

// googleAsk is what one sign-in carries to Google and must find again when the person comes back: the state that ties the
// answer to this browser, the nonce Google signs into its token, and the PKCE verifier only this server holds.
type googleAsk struct{ state, nonce, verifier string }

// googlePerson is who Google says came back: an address Google speaks for, and the first name the person goes by, which
// names a workspace that no company's domain names.
type googlePerson struct{ email, name string }

// googleWho exchanges the code Google sent back and reads the person out of the token Google signed, checking that
// signature, that the token answers this sign-in, and that Google may speak for the address.
func (g *Google) googleWho(ctx context.Context, code string, ask googleAsk) (googlePerson, error) {
	ctx, cancel := context.WithTimeout(ctx, googleWait)
	defer cancel()
	token, err := g.config.Exchange(ctx, code, oauth2.VerifierOption(ask.verifier))
	if err != nil {
		return googlePerson{}, err
	}
	raw, carried := token.Extra("id_token").(string)
	if !carried {
		return googlePerson{}, errors.New("Google returned no identity token")
	}
	checked, err := g.verifier.Verify(ctx, raw)
	if err != nil {
		return googlePerson{}, err
	}
	if subtle.ConstantTimeCompare([]byte(checked.Nonce), []byte(ask.nonce)) != 1 {
		return googlePerson{}, errors.New("the identity token was not issued for this sign-in")
	}
	var claims struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
		Hosted   string `json:"hd"`
		Given    string `json:"given_name"`
		Name     string `json:"name"`
	}
	if err := checked.Claims(&claims); err != nil {
		return googlePerson{}, err
	}
	email, err := NormalEmail(claims.Email)
	if err != nil || !claims.Verified || !googleOwns(email, claims.Hosted) {
		return googlePerson{}, fmt.Errorf("Google does not speak for %q", claims.Email)
	}
	name := claims.Given
	if strings.TrimSpace(name) == "" {
		name = claims.Name
	}
	return googlePerson{email: email, name: name}, nil
}

// googleOwns says whether Google is the authority for an address: a Gmail address, or one of the Google Workspace domain the
// token names. A Google account made with another company's address had that address checked once, when it was made, and is
// not taken as proof that its owner still reads it.
func googleOwns(email, hosted string) bool {
	_, domain, _ := strings.Cut(email, "@")
	return domain == "gmail.com" || domain == "googlemail.com" || (hosted != "" && strings.EqualFold(hosted, domain))
}

func (s *Server) googleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /signin/google", s.googleStart)
	mux.HandleFunc("GET "+GoogleBack, s.googleBack)
}

// googleStart sends a person to Google, carrying what must come back with them.
func (s *Server) googleStart(w http.ResponseWriter, r *http.Request) {
	if s.Google == nil {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return
	}
	ask := googleAsk{state: randomText(), nonce: randomText(), verifier: oauth2.GenerateVerifier()}
	if ask.state == "" || ask.nonce == "" {
		http.Error(w, "could not begin signing in", http.StatusInternalServerError)
		return
	}
	s.Sessions.keepAsk(w, ask)
	http.Redirect(w, r, s.Google.config.AuthCodeURL(ask.state, oauth2.S256ChallengeOption(ask.verifier),
		oauth2.SetAuthURLParam("nonce", ask.nonce), oauth2.SetAuthURLParam("prompt", "select_account")), http.StatusSeeOther)
}

// googleBack receives a person Google sends back. One with an account is signed in, and anybody else signs up as the address
// Google confirmed, which needs no mail to confirm it.
func (s *Server) googleBack(w http.ResponseWriter, r *http.Request) {
	if s.Google == nil {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return
	}
	ask, kept := s.Sessions.takeAsk(w, r)
	query := r.URL.Query()
	if !kept || subtle.ConstantTimeCompare([]byte(ask.state), []byte(query.Get("state"))) != 1 {
		s.signInRefused(w, http.StatusBadRequest, "That sign-in did not start here. Try again.")
		return
	}
	if query.Get("error") != "" {
		s.signInRefused(w, http.StatusOK, "Google sign-in was cancelled.")
		return
	}
	if !s.signups.Allow(r.Context(), "signin:"+s.source(r)) {
		s.signInRefused(w, http.StatusTooManyRequests, "That is a lot of attempts from one place. Try again later.")
		return
	}
	who, err := s.Google.googleWho(r.Context(), query.Get("code"), ask)
	if err != nil {
		slog.Warn("a Google sign-in was refused", "error", err)
		s.signInRefused(w, http.StatusBadRequest, "Google could not confirm a work address gusto can use. Sign in with your email instead.")
		return
	}
	found, err := s.Store.HasAccount(r.Context(), who.email)
	if err != nil {
		slog.Error("an account could not be looked up", "error", err)
		s.signInRefused(w, http.StatusInternalServerError, "Something went wrong on our side. Try again.")
		return
	}
	if found {
		s.confirmed(r.Context(), who.email)
		s.signedIn(w, r, who.email)
		return
	}
	s.googleSignUp(w, r, who)
}

// googleSignUp makes the organization of a person Google confirmed: their company's, or a workspace of their own named
// after them. Where their company has an organization already, the sign-up form asks for a name for this one, and
// remembers for a while that Google confirmed the address.
func (s *Server) googleSignUp(w http.ResponseWriter, r *http.Request, who googlePerson) {
	email := who.email
	if !s.all.Allow(r.Context(), "all") {
		s.render(w, http.StatusServiceUnavailable, "signup", view{Title: "Get started", Email: email,
			Error: "Sign-ups are busy right now. Try again in a little while."})
		return
	}
	person, org, err := s.Store.SignUp(r.Context(), Signup{Email: email, Person: who.name})
	if err != nil {
		more := asksForMore(err)
		if more {
			s.Sessions.Vouch(w, email)
		}
		page, status := view{Title: "Get started", Email: email, Error: refusalText(err), More: more}, refusalStatus(err)
		if ask := nextQuestion(err, Signup{Email: email}); ask != "" {
			status, page.Error, page.Ask = http.StatusOK, "", ask
		}
		s.render(w, status, "signup", page)
		return
	}
	s.confirmed(r.Context(), person.Email)
	s.welcome(w, r, person, org)
}

func (s *Server) signInRefused(w http.ResponseWriter, status int, why string) {
	s.render(w, status, "signin", view{Title: "Sign in", Error: why})
}

// keepAsk keeps what a sign-in carries to Google in this browser for ten minutes, which is long enough to choose an account.
func (s *Sessions) keepAsk(w http.ResponseWriter, ask googleAsk) {
	http.SetCookie(w, &http.Cookie{Name: s.googleName(), Value: ask.state + "." + ask.nonce + "." + ask.verifier, Path: "/",
		MaxAge: int((10 * time.Minute) / time.Second), HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode})
}

// takeAsk is what keepAsk kept, which is spent whatever happens next, so each sign-in is one try.
func (s *Sessions) takeAsk(w http.ResponseWriter, r *http.Request) (googleAsk, bool) {
	http.SetCookie(w, &http.Cookie{Name: s.googleName(), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode})
	cookie, err := r.Cookie(s.googleName())
	if err != nil {
		return googleAsk{}, false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return googleAsk{}, false
	}
	return googleAsk{state: parts[0], nonce: parts[1], verifier: parts[2]}, true
}

func (s *Sessions) googleName() string {
	if s.secure {
		return "__Host-gusto_google"
	}
	return "gusto_google"
}

// randomText is 24 random bytes as text, or nothing when none can be had.
func randomText() string {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
