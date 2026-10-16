package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	neturl "net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Identity is somebody signing in with an identity provider the deployment
// names, rather than with a password everybody shares.
//
// It is deliberately a provider this server points at rather than one it is:
// a customer running this in their own cloud points it at their own, and
// "sovereign, except sign-in, which calls somebody else" is not a claim that
// survives a review.
type Identity struct {
	provider *oidc.Provider
	config   oauth2.Config
	verifier *oidc.IDTokenVerifier
	// access is what turns who somebody is into whether they may spend this
	// server's credential, and as which role.
	access Access
}

// Configured reports whether an identity provider was named. Without one the
// shared password is the only way in, which is what a deployment with no
// provider has.
func (i *Identity) Configured() bool { return i != nil && i.provider != nil }

// NewIdentity reaches the provider's discovery document, so a wrong issuer
// fails at startup rather than at somebody's first sign-in.
func NewIdentity(ctx context.Context, issuer, id, secret, redirect string, access Access) (*Identity, error) {
	if issuer == "" {
		return &Identity{}, nil
	}
	if id == "" || secret == "" || redirect == "" {
		return nil, errors.New("an identity provider needs a client id, a secret and a redirect address")
	}
	if !access.gated() {
		return nil, errUngated
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("could not reach %s: %w", issuer, err)
	}
	return &Identity{
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: id}),
		config: oauth2.Config{
			ClientID:     id,
			ClientSecret: secret,
			RedirectURL:  redirect,
			Endpoint:     provider.Endpoint(),
			Scopes:       append([]string{oidc.ScopeOpenID, "email"}, access.Scopes...),
		},
		access: access,
	}, nil
}

// start sends somebody to their provider, carrying a value that comes back with
// them so an answer nobody here asked for is refused.
func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	if !s.identity.Configured() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	state, nonce := randomValue(), randomValue()
	if state == "" || nonce == "" {
		http.Error(w, "could not begin signing in", http.StatusInternalServerError)
		return
	}
	// The verifier stays here and only its hash goes to the provider, so a code
	// that is intercepted on its way back cannot be traded for a token by
	// anybody who lacks it. The nonce comes back inside the signed token, so a
	// token the provider issued for another sign-in is not accepted for this one.
	verifier := oauth2.GenerateVerifier()
	for name, value := range map[string]string{stateCookie: state, verifierCookie: verifier, nonceCookie: nonce} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: value, Path: "/auth", HttpOnly: true,
			SameSite: http.SameSiteLaxMode, Secure: overTLS(r), Expires: time.Now().Add(10 * time.Minute),
		})
	}
	http.Redirect(w, r, s.identity.config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("nonce", nonce)), http.StatusSeeOther)
}

// back receives somebody returning from their provider.
func (s *Server) back(w http.ResponseWriter, r *http.Request) {
	if !s.identity.Configured() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	given, err := r.Cookie(stateCookie)
	if err != nil || given.Value == "" || subtle.ConstantTimeCompare([]byte(given.Value), []byte(r.URL.Query().Get("state"))) != 1 {
		// An answer this server did not ask for, so it is not acted on.
		s.noteRefused(r, "", "the provider", "the answer did not come from a sign-in started here")
		refuse(w, r, "that sign-in did not start here")
		return
	}
	verifier, nonce := cookieValue(r, verifierCookie), cookieValue(r, nonceCookie)
	// Each sign-in is one try: whatever happens next, the values are spent.
	for _, name := range []string{stateCookie, verifierCookie, nonceCookie} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/auth", MaxAge: -1})
	}
	who, err := s.identity.person(r.Context(), r.URL.Query().Get("code"), verifier, nonce)
	if err != nil {
		slog.Warn("a sign-in could not be checked", "error", err)
		s.noteRefused(r, "", "the provider", "the sign-in could not be checked")
		refuse(w, r, "that sign-in could not be checked")
		return
	}
	role, admitted := s.roleFor(who)
	if !admitted {
		s.noteRefused(r, who.Email, "the provider", "no role maps to this person")
		// Who they are is settled; whether they may spend this server's
		// credential is a separate question, and the answer is no.
		slog.Info("a sign-in was refused: no role maps to this person", "email", who.Email, "groups", len(who.Groups))
		refuse(w, r, who.Email+" is not allowed here")
		return
	}
	s.keeper.issueFor(w, r, who.Email, role)
	s.noteSignIn(r, who.Email, role, "the provider")
	s.afterSignIn(w, r)
}

func refuse(w http.ResponseWriter, r *http.Request, why string) {
	http.Redirect(w, r, "/login?refused="+neturl.QueryEscape(why), http.StatusSeeOther)
}

// person exchanges the code and reads who signed in out of the token the
// provider signed, verifying that signature rather than trusting the answer.
func (i *Identity) person(ctx context.Context, code, verifier, nonce string) (Person, error) {
	if verifier == "" || nonce == "" {
		return Person{}, errors.New("the sign-in carried no verifier")
	}
	token, err := i.config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Person{}, err
	}
	raw, carried := token.Extra("id_token").(string)
	if !carried {
		return Person{}, errors.New("the provider returned no identity token")
	}
	checked, err := i.verifier.Verify(ctx, raw)
	if err != nil {
		return Person{}, err
	}
	if subtle.ConstantTimeCompare([]byte(checked.Nonce), []byte(nonce)) != 1 {
		return Person{}, errors.New("the identity token was not issued for this sign-in")
	}
	var claims map[string]json.RawMessage
	if err := checked.Claims(&claims); err != nil {
		return Person{}, err
	}
	var email string
	var verified bool
	json.Unmarshal(claims["email"], &email)
	json.Unmarshal(claims["email_verified"], &verified)
	if email == "" || !verified {
		return Person{}, errors.New("the provider did not confirm an address")
	}
	return Person{Email: email, Groups: groupsIn(claims[i.access.groupsClaim()])}, nil
}

func overTLS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

const (
	stateCookie    = "gusto-state"
	verifierCookie = "gusto-verifier"
	nonceCookie    = "gusto-nonce"
)

// randomValue is 24 random bytes as text, or nothing when none can be had.
func randomValue() string {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func cookieValue(r *http.Request, name string) string {
	if cookie, err := r.Cookie(name); err == nil {
		return cookie.Value
	}
	return ""
}
