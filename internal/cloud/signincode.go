package cloud

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// SignInCode is six digits mailed beside a sign-in link, for a person who reads mail on one device and signs in on another.
const SignInCode Purpose = "signin-code"

// codeTries is the most codes one address may be tried with in an hour. Six digits are a million codes, so a guesser has
// five in a million chances at each address an hour.
const codeTries = 5

// codeKey is what a code is kept as: the hash of the address with the code, so a code works only for the address it was
// mailed to, and the same digits for two addresses are two codes.
func codeKey(email, code string) string { return "code:" + email + ":" + code }

// MakeSignInCode is six digits that sign the address in once, for as long as a sign-in link lasts.
func (s *Store) MakeSignInCode(email string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	made := link{Email: email, Purpose: SignInCode, Expires: s.now().Add(SignInLife)}
	_, err = s.db.ExecContext(context.Background(), insertLink.SQL(1), s.linkArgs(digest(codeKey(email, code)), made)...)
	return code, err
}

// UseSignInCode spends the code the address was mailed. A code typed with spaces or a dash between its digits is the same code.
func (s *Store) UseSignInCode(email, typed string) error {
	code := strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(typed))
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return ErrLink
	}
	_, err := s.UseLink(codeKey(email, code), SignInCode)
	return err
}

// forgetSignIns spends every sign-in link and code an address has, once one of them has signed it in.
func (s *Store) forgetSignIns(email string) error {
	_, err := s.db.ExecContext(context.Background(), `DELETE FROM links WHERE email = ? AND purpose IN (?, ?)`, email, string(SignIn), string(SignInCode))
	return err
}

// signInWithCode signs a person in with the code they were mailed. Wrong codes are counted for the address and for where
// they come from, and an address with no account is answered as one whose code is wrong.
func (s *Server) signInWithCode(w http.ResponseWriter, r *http.Request) {
	email, err := NormalEmail(r.PostFormValue("email"))
	if err != nil {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return
	}
	again := func(status int, why string) {
		s.render(w, status, "signin", view{Title: "Check your email", Email: email, Sent: true, Error: why})
	}
	if !s.signups.Allow("code:"+s.source(r)) || !s.codes.Allow(email) {
		again(http.StatusTooManyRequests, "That is a lot of tries. Use the link in the email, or ask for a new code in a while.")
		return
	}
	if err := s.Store.UseSignInCode(email, r.PostFormValue("code")); err != nil {
		again(http.StatusUnprocessableEntity, "That code is wrong or has expired. Check the newest email from gusto, or ask for a new one.")
		return
	}
	s.signedIn(w, r, email)
}

// signedIn starts a person's session once a link or code showed the address is theirs, and spends the rest they were sent.
func (s *Server) signedIn(w http.ResponseWriter, r *http.Request, email string) {
	if err := s.Store.forgetSignIns(email); err != nil {
		log.Printf("cloud: the other sign-in links of an address were not spent: %v", err)
	}
	s.Sessions.Issue(w, email)
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

// signInMail is what a person who asked to sign in is sent: the code, and the link for the device they read mail on.
func signInMail(code, address string, life time.Duration) (subject, body string) {
	minutes := int(life / time.Minute)
	return "Your gusto sign-in code is " + code,
		"Your sign-in code is " + code + "\n\nType it on the page where you asked to sign in. Or follow this link to sign in on this device:\n\n" +
			address + fmt.Sprintf("\n\nThe code and the link work once and expire in %d minutes. If you did not ask for them, ignore this mail.\n", minutes)
}
