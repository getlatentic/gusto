package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
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
	// Allowed is the addresses and domains that may sign in. A provider says
	// who somebody is; it does not say whether they may spend this server's
	// credential.
	allowed []string
}

// Configured reports whether an identity provider was named. Without one the
// shared password is the only way in, which is what a deployment with no
// provider has.
func (i *Identity) Configured() bool { return i != nil && i.provider != nil }

// NewIdentity reaches the provider's discovery document, so a wrong issuer
// fails at startup rather than at somebody's first sign-in.
func NewIdentity(ctx context.Context, issuer, id, secret, redirect string, allowed []string) (*Identity, error) {
	if issuer == "" {
		return &Identity{}, nil
	}
	if id == "" || secret == "" || redirect == "" {
		return nil, errors.New("an identity provider needs a client id, a secret and a redirect address")
	}
	if len(allowed) == 0 {
		return nil, errors.New("name who may sign in, or anybody with an account at that provider can spend this server's credential")
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
			Scopes:       []string{oidc.ScopeOpenID, "email"},
		},
		allowed: allowed,
	}, nil
}

// Admits reports whether this address may sign in. An entry is an address, or a
// domain that any address at it may use.
func (i *Identity) Admits(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	_, domain, found := strings.Cut(email, "@")
	for _, allowed := range i.allowed {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		switch {
		case allowed == "":
		case allowed == email:
			return true
		case found && strings.HasPrefix(allowed, "@") && allowed[1:] == domain:
			return true
		}
	}
	return false
}

// start sends somebody to their provider, carrying a value that comes back with
// them so an answer nobody here asked for is refused.
func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	if !s.identity.Configured() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		http.Error(w, "could not begin signing in", http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(raw)
	http.SetCookie(w, &http.Cookie{
		Name: "haas-state", Value: state, Path: "/auth", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: overTLS(r), Expires: time.Now().Add(10 * time.Minute),
	})
	http.Redirect(w, r, s.identity.config.AuthCodeURL(state), http.StatusSeeOther)
}

// back receives somebody returning from their provider.
func (s *Server) back(w http.ResponseWriter, r *http.Request) {
	if !s.identity.Configured() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	given, err := r.Cookie("haas-state")
	if err != nil || given.Value == "" || given.Value != r.URL.Query().Get("state") {
		// An answer this server did not ask for, so it is not acted on.
		refuse(w, r, "that sign-in did not start here")
		return
	}
	email, err := s.identity.who(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		refuse(w, r, "that sign-in could not be checked")
		return
	}
	if !s.identity.Admits(email) {
		// Who they are is settled; whether they may spend this server's
		// credential is a separate question, and the answer is no.
		refuse(w, r, email+" is not allowed here")
		return
	}
	s.keeper.issueFor(w, r, email)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func refuse(w http.ResponseWriter, r *http.Request, why string) {
	http.Redirect(w, r, "/login?refused="+neturl.QueryEscape(why), http.StatusSeeOther)
}

// who exchanges the code and reads the address out of the token the provider
// signed, verifying that signature rather than trusting the answer.
func (i *Identity) who(ctx context.Context, code string) (string, error) {
	token, err := i.config.Exchange(ctx, code)
	if err != nil {
		return "", err
	}
	raw, carried := token.Extra("id_token").(string)
	if !carried {
		return "", errors.New("the provider returned no identity token")
	}
	checked, err := i.verifier.Verify(ctx, raw)
	if err != nil {
		return "", err
	}
	var claims struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
	}
	if err := checked.Claims(&claims); err != nil {
		return "", err
	}
	if claims.Email == "" || !claims.Verified {
		return "", errors.New("the provider did not confirm an address")
	}
	return claims.Email, nil
}

func overTLS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}
