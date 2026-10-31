package cloud

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// GitHub signs people in with their GitHub account: the OAuth app the operator registered, whose callback URLs include this front
// door's /signin/github/callback. A person is signed in as an address GitHub says they verified, so an account here is never
// reached through an address its owner has not shown is theirs.
type GitHub struct {
	ID, Secret string
	// HTTP calls GitHub. It has a timeout of ten seconds when it is not set.
	HTTP *http.Client
	// Site and API are github.com's addresses, and another's in a test.
	Site, API string
}

// githubAsk is how long a sign-in at GitHub may take before the browser comes back, and the cookie that waits for it lasts.
const githubAsk = 10 * time.Minute

// githubAnswer is the most of GitHub's answer that is read.
const githubAnswer = 1 << 20

func (g *GitHub) site() string { return strings.TrimSuffix(cmp.Or(g.Site, "https://github.com"), "/") }
func (g *GitHub) api() string  { return strings.TrimSuffix(cmp.Or(g.API, "https://api.github.com"), "/") }

func (g *GitHub) client() *http.Client {
	if g.HTTP != nil {
		return g.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// githubCookie holds the state and the PKCE verifier of a sign-in at GitHub until the browser comes back.
func (s *Sessions) githubCookie() string {
	if s.secure {
		return "__Host-gusto_github"
	}
	return "gusto_github"
}

func (s *Server) githubCallback() string { return s.URL.JoinPath("/signin/github/callback").String() }

// startGitHub sends a person to GitHub to sign in, with a state and a PKCE challenge only this browser can answer.
func (s *Server) startGitHub(w http.ResponseWriter, r *http.Request) {
	if s.GitHub == nil {
		http.NotFound(w, r)
		return
	}
	if !s.signups.Allow("github:" + s.source(r)) {
		s.render(w, http.StatusTooManyRequests, "signin", view{Title: "Sign in", Error: "That is a lot of attempts from one place. Try again later."})
		return
	}
	state, verifier := rand.Text(), rand.Text()+rand.Text()
	http.SetCookie(w, &http.Cookie{Name: s.Sessions.githubCookie(), Value: state + "." + verifier, Path: "/signin/github",
		MaxAge: int(githubAsk / time.Second), HttpOnly: true, Secure: s.Sessions.secure, SameSite: http.SameSiteLaxMode})
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"client_id": {s.GitHub.ID}, "redirect_uri": {s.githubCallback()}, "scope": {"user:email"}, "state": {state},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"},
	}
	http.Redirect(w, r, s.GitHub.site()+"/login/oauth/authorize?"+query.Encode(), http.StatusSeeOther)
}

// finishGitHub is where GitHub sends a person back to. They are signed in as the first address GitHub verified that has an
// account here, their primary one first. A person with none is asked to make an organization, with an address GitHub verified
// written in, and is not mailed to confirm it.
func (s *Server) finishGitHub(w http.ResponseWriter, r *http.Request) {
	if s.GitHub == nil {
		http.NotFound(w, r)
		return
	}
	refuse := func(why string) {
		s.render(w, http.StatusBadRequest, "signin", view{Title: "Sign in", Error: why})
	}
	verifier, ok := s.githubVerifier(w, r)
	if !ok {
		refuse("That sign-in with GitHub was not started here, or took too long. Try again.")
		return
	}
	if why := r.URL.Query().Get("error"); why != "" {
		refuse("GitHub did not sign you in: " + cmp.Or(r.URL.Query().Get("error_description"), why) + ".")
		return
	}
	emails, err := s.GitHub.verifiedEmails(r.Context(), r.URL.Query().Get("code"), verifier, s.githubCallback())
	if err != nil {
		log.Printf("cloud: a sign-in with GitHub failed: %v", err)
		refuse("GitHub could not be asked who you are. Try again.")
		return
	}
	for _, email := range emails {
		if found, err := s.Store.HasAccount(email); err == nil && found {
			normal, _ := NormalEmail(email)
			s.Sessions.Issue(w, normal)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	email, found := s.signUpEmail(emails)
	if !found {
		refuse("GitHub has no verified email address for you. Add one at GitHub, or sign in with your email here.")
		return
	}
	s.Sessions.Vouch(w, email)
	s.render(w, http.StatusOK, "signup", view{Title: "Create your organization", Email: email,
		Notice: "GitHub verified " + email + ". Create your organization with it, or with another work address."})
}

// githubVerifier is the PKCE verifier of the sign-in the request comes back from, when its state is the one this browser was
// given. The cookie is spent either way.
func (s *Server) githubVerifier(w http.ResponseWriter, r *http.Request) (string, bool) {
	cookie, err := r.Cookie(s.Sessions.githubCookie())
	http.SetCookie(w, &http.Cookie{Name: s.Sessions.githubCookie(), Value: "", Path: "/signin/github", MaxAge: -1,
		HttpOnly: true, Secure: s.Sessions.secure, SameSite: http.SameSiteLaxMode})
	if err != nil {
		return "", false
	}
	state, verifier, found := strings.Cut(cookie.Value, ".")
	given := r.URL.Query().Get("state")
	return verifier, found && state != "" && subtle.ConstantTimeCompare([]byte(state), []byte(given)) == 1
}

// signUpEmail is the address a person who has no account is asked to sign up with: the first verified one an organization
// may be made with, their primary one first, or their primary one when none may.
func (s *Server) signUpEmail(emails []string) (string, bool) {
	for _, email := range emails {
		if s.Store.MaySignUp(email) == nil {
			return email, true
		}
	}
	if len(emails) == 0 {
		return "", false
	}
	return emails[0], true
}

// githubEmail is one of a person's addresses as GitHub lists them.
type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// verifiedEmails trades the code GitHub gave for a token, and reads with it the addresses the person verified at GitHub,
// their primary one first. The token is not kept.
func (g *GitHub) verifiedEmails(ctx context.Context, code, verifier, callback string) ([]string, error) {
	if code == "" {
		return nil, errors.New("GitHub sent no code")
	}
	token, err := g.token(ctx, code, verifier, callback)
	if err != nil {
		return nil, err
	}
	asking, err := http.NewRequestWithContext(ctx, http.MethodGet, g.api()+"/user/emails", nil)
	if err != nil {
		return nil, err
	}
	asking.Header.Set("Authorization", "Bearer "+token)
	asking.Header.Set("Accept", "application/vnd.github+json")
	asking.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	var listed []githubEmail
	if err := g.call(asking, &listed); err != nil {
		return nil, fmt.Errorf("the addresses: %w", err)
	}
	slices.SortStableFunc(listed, func(a, b githubEmail) int {
		if a.Primary == b.Primary {
			return 0
		}
		if a.Primary {
			return -1
		}
		return 1
	})
	var verified []string
	for _, each := range listed {
		if each.Verified && !strings.HasSuffix(strings.ToLower(each.Email), "@users.noreply.github.com") {
			if normal, err := NormalEmail(each.Email); err == nil {
				verified = append(verified, normal)
			}
		}
	}
	return verified, nil
}

func (g *GitHub) token(ctx context.Context, code, verifier, callback string) (string, error) {
	form := url.Values{"client_id": {g.ID}, "client_secret": {g.Secret}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {verifier}}
	asking, err := http.NewRequestWithContext(ctx, http.MethodPost, g.site()+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	asking.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	asking.Header.Set("Accept", "application/json")
	var answer struct {
		Token       string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := g.call(asking, &answer); err != nil {
		return "", fmt.Errorf("the token: %w", err)
	}
	if answer.Token == "" {
		return "", fmt.Errorf("the token was refused: %s %s", answer.Error, answer.Description)
	}
	return answer.Token, nil
}

// call sends a request to GitHub and reads its JSON answer, which GitHub sends with 200 even when it refuses a code.
func (g *GitHub) call(asking *http.Request, into any) error {
	answered, err := g.client().Do(asking)
	if err != nil {
		return err
	}
	defer answered.Body.Close()
	body, err := io.ReadAll(io.LimitReader(answered.Body, githubAnswer))
	if err != nil {
		return err
	}
	if answered.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub answered %d", answered.StatusCode)
	}
	return json.Unmarshal(body, into)
}
