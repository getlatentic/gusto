package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// A shared password stands in for an identity provider. It is the smallest
// thing that stops a URL which spends money and runs code being open to
// whoever finds it, and the shape does not block SSO later: every handler
// already asks "who is this" rather than "is this on".
type keeper struct {
	password string
	key      []byte
}

func newKeeper(password string) (*keeper, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	// The key is made at boot, so a restart asks everybody to sign in again.
	// That is the honest trade for keeping no secret on disk.
	return &keeper{password: password, key: key}, nil
}

const cookieName = "haas"

func (k *keeper) admits(given string) bool {
	return subtle.ConstantTimeCompare([]byte(given), []byte(k.password)) == 1
}

// issue marks the cookie Secure when the request arrived over TLS, directly or
// through a proxy that terminated it. Marking it always would mean the cookie is
// never sent back over plain http, which is how a local run works.
func (k *keeper) issue(w http.ResponseWriter, r *http.Request) {
	until := time.Now().Add(12 * time.Hour).Unix()
	value := strconv.FormatInt(until, 10)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    value + "." + k.sign(value),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		Expires:  time.Unix(until, 0),
	})
}

func (k *keeper) clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
}

// knows reports whether this request carries a signature this server made and
// has not yet expired.
func (k *keeper) knows(r *http.Request) bool {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	value, signature, split := strings.Cut(cookie.Value, ".")
	if !split || !hmac.Equal([]byte(signature), []byte(k.sign(value))) {
		return false
	}
	until, err := strconv.ParseInt(value, 10, 64)
	return err == nil && time.Now().Unix() < until
}

func (k *keeper) sign(value string) string {
	mac := hmac.New(sha256.New, k.key)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// guard sends anybody this server does not know to the sign-in page, and never
// to the thing they asked for.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.keeper.knows(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}
