package cloud

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Sessions are signed cookies that hold an address and a time, and nothing else.
// What a person may do is read from the store on each request, so removing them
// takes effect at once. The key is kept in a file, so a restart signs nobody out.
type Sessions struct {
	key    []byte
	secure bool
	life   time.Duration
	now    func() time.Time
}

// SessionLife is how long a sign-in lasts.
const SessionLife = 30 * 24 * time.Hour

// OpenSessions reads the signing key in root, and makes it when there is none.
// secure is whether the cookie is for a site served over https.
func OpenSessions(root string, secure bool) (*Sessions, error) {
	path := filepath.Join(root, "secret")
	key, err := os.ReadFile(path)
	if missing(err) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, err = file.Write(key)
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
		}
		if os.IsExist(err) {
			key, err = os.ReadFile(path)
		}
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) < 32 {
		return nil, errors.New("the front door's signing key is too short: " + path)
	}
	return &Sessions{key: key, secure: secure, life: SessionLife, now: time.Now}, nil
}

// name is the cookie's name. Over https it is a host-locked one, which a
// sibling subdomain cannot set: an organisation's own server is one.
func (s *Sessions) name() string {
	if s.secure {
		return "__Host-haas_cloud"
	}
	return "haas_cloud"
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

// Clear signs a person out.
func (s *Sessions) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: s.name(), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode})
}
