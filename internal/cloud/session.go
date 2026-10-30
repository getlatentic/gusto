package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Sessions are signed cookies that hold an address and a time, and nothing else, and beside them
// the organization a person chose when they are in several.
// What a person may do is read from the store on each request, so removing them
// takes effect at once. The key is kept in the database, so a restart signs nobody out.
type Sessions struct {
	key    []byte
	secure bool
	life   time.Duration
	now    func() time.Time
}

// SessionLife is how long a sign-in lasts.
const SessionLife = 30 * 24 * time.Hour

// OpenSessions reads the signing key the database holds, and makes it when there is none, so that
// every front door on one database signs and checks the same cookies and a restart signs nobody out.
// secure is whether the cookie is for a site served over https.
func (s *Store) OpenSessions(secure bool) (*Sessions, error) {
	key, err := s.sessionKey()
	if err != nil {
		return nil, err
	}
	return &Sessions{key: key, secure: secure, life: SessionLife, now: time.Now}, nil
}

func (s *Store) sessionKey() ([]byte, error) {
	if key, err := s.storedKey(); err != nil || key != nil {
		return key, err
	}
	fresh := make([]byte, 32)
	if _, err := rand.Read(fresh); err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(context.Background(), keepSetting.SQL(1), settingSessionKey, hex.EncodeToString(fresh)); err != nil {
		return nil, err
	}
	key, err := s.storedKey()
	if err == nil && key == nil {
		err = errors.New("gusto cloud's signing key was not kept")
	}
	return key, err
}

func (s *Store) storedKey() ([]byte, error) {
	text, found, err := s.setting(settingSessionKey)
	if err != nil || !found {
		return nil, err
	}
	key, err := hex.DecodeString(text)
	if err != nil || len(key) < 32 {
		return nil, errors.New("gusto cloud's signing key in the database is too short or damaged")
	}
	return key, nil
}

// name is the cookie's name. Over https it is a host-locked one, which a
// sibling subdomain cannot set: an organization's own server is one.
func (s *Sessions) name() string {
	if s.secure {
		return "__Host-gusto_cloud"
	}
	return "gusto_cloud"
}

func (s *Sessions) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Issue signs a person in.
func (s *Sessions) Issue(w http.ResponseWriter, email string) {
	expires := s.now().Add(s.life)
	payload := base64.RawURLEncoding.EncodeToString([]byte(email + "|" + strconv.FormatInt(expires.Unix(), 10)))
	http.SetCookie(w, &http.Cookie{Name: s.name(), Value: payload + "." + s.sign(payload), Path: "/",
		Expires: expires, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode})
}

// Who is the address a request is signed in as.
func (s *Sessions) Who(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(s.name())
	if err != nil {
		return "", false
	}
	payload, signature, found := strings.Cut(cookie.Value, ".")
	if !found || !hmac.Equal([]byte(signature), []byte(s.sign(payload))) {
		return "", false
	}
	text, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", false
	}
	email, until, found := strings.Cut(string(text), "|")
	seconds, err := strconv.ParseInt(until, 10, 64)
	if !found || err != nil || !s.now().Before(time.Unix(seconds, 0)) {
		return "", false
	}
	return email, true
}

// Clear signs a person out, and forgets which of their organizations they were in.
func (s *Sessions) Clear(w http.ResponseWriter) {
	for _, name := range []string{s.name(), s.orgName()} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode})
	}
}

// orgName is the cookie that names which of their organizations a person is in. It is not signed: it is
// only a choice, and the store says on every request whether the person is in the organization it names.
func (s *Sessions) orgName() string {
	if s.secure {
		return "__Host-gusto_org"
	}
	return "gusto_org"
}

// Choose remembers the organization a person is in, until they choose another.
func (s *Sessions) Choose(w http.ResponseWriter, org string) {
	http.SetCookie(w, &http.Cookie{Name: s.orgName(), Value: org, Path: "/",
		Expires: s.now().Add(s.life), HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode})
}

// Chosen is the organization a request says its person is in, which may be none.
func (s *Sessions) Chosen(r *http.Request) string {
	cookie, err := r.Cookie(s.orgName())
	if err != nil {
		return ""
	}
	return cookie.Value
}

// joinedName is the cookie that says which invitation this browser just used, so that pressing Join again goes in.
func (s *Sessions) joinedName() string {
	if s.secure {
		return "__Host-gusto_joined"
	}
	return "gusto_joined"
}

// Joined remembers, for a few minutes, that this browser used an invitation. Only a hash of its link is kept.
func (s *Sessions) Joined(w http.ResponseWriter, secret string) {
	http.SetCookie(w, &http.Cookie{Name: s.joinedName(), Value: linkHash(secret), Path: "/", MaxAge: int(joinedFor / time.Second),
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode})
}

// JoinedWith says whether this browser is the one that just used the invitation.
func (s *Sessions) JoinedWith(r *http.Request, secret string) bool {
	cookie, err := r.Cookie(s.joinedName())
	return err == nil && secret != "" && hmac.Equal([]byte(cookie.Value), []byte(linkHash(secret)))
}

// joinedFor is how long a browser that used an invitation is let in again by its link.
const joinedFor = 10 * time.Minute

func linkHash(secret string) string {
	sum := sha256.Sum256([]byte("joined:" + secret))
	return hex.EncodeToString(sum[:])
}

// ownCookie says whether a cookie is the front door's, over https or not, which no organization's server is sent.
func ownCookie(name string) bool {
	switch name {
	case "gusto_cloud", "__Host-gusto_cloud", "gusto_org", "__Host-gusto_org", "gusto_joined", "__Host-gusto_joined":
		return true
	}
	return false
}
