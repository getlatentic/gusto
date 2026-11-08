// Package entry is how a hosted organization's server knows who a person is
// without a sign-in page of its own. Gusto cloud signs a person in, and then
// hands their browser a token, good for a minute and for one use, that the
// organization's server checks against a key only the two of them share.
package entry

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

// Claims are what gusto cloud says: who this is, what they may do in the
// organization, which organization it is, and until when the word is good.
// Purpose is empty on a token to enter with, and exit on the one an
// organization's server hands back when the person signs out, so neither
// works as the other.
type Claims struct {
	Email   string `json:"e"`
	Role    string `json:"r"`
	Org     string `json:"o"`
	Expires int64  `json:"x"`
	Nonce   string `json:"n"`
	Purpose string `json:"p,omitempty"`
}

const exit = "exit"

// Life is how long a token works. A browser follows it at once.
const Life = time.Minute

// ErrInvalid is every reason a token does not work, said once.
var ErrInvalid = errors.New("that sign-in link has been used or has expired")

// Mint is a token for a person to enter an organization with.
func Mint(key []byte, org, email, role string, now time.Time) (string, error) {
	return mint(key, Claims{Email: email, Role: role, Org: org}, now)
}

// MintExit is the token an organization's server sends a person's browser to
// gusto cloud with when they sign out, so that gusto cloud signs them out too.
func MintExit(key []byte, org, email string, now time.Time) (string, error) {
	return mint(key, Claims{Email: email, Org: org, Purpose: exit}, now)
}

func mint(key []byte, claims Claims, now time.Time) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	claims.Expires = now.Add(Life).Unix()
	claims.Nonce = base64.RawURLEncoding.EncodeToString(nonce)
	text, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(text)
	return payload + "." + sign(key, payload), nil
}

func sign(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Verify is the claims in a token, when it was made with the key, is for this
// organization, and has not run out. It does not say the token is unused: that
// is Seen's.
func Verify(key []byte, org, token string, now time.Time) (Claims, error) {
	return verify(key, org, token, "", now)
}

// VerifyExit is the claims in a token MintExit made, on the same terms.
func VerifyExit(key []byte, org, token string, now time.Time) (Claims, error) {
	return verify(key, org, token, exit, now)
}

func verify(key []byte, org, token, purpose string, now time.Time) (Claims, error) {
	payload, signature, found := strings.Cut(token, ".")
	if !found || !hmac.Equal([]byte(signature), []byte(sign(key, payload))) {
		return Claims{}, ErrInvalid
	}
	text, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Claims{}, ErrInvalid
	}
	var claims Claims
	if err := json.Unmarshal(text, &claims); err != nil || claims.Org != org || claims.Purpose != purpose || claims.Email == "" || claims.Nonce == "" || !now.Before(time.Unix(claims.Expires, 0)) {
		return Claims{}, ErrInvalid
	}
	return claims, nil
}

// Seen remembers the tokens that were used until each would have run out anyway,
// so one cannot be used twice.
type Seen struct {
	mu   sync.Mutex
	used map[string]int64
}

// Use spends a token's nonce, and says whether it was unspent.
func (s *Seen) Use(claims Claims, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used == nil {
		s.used = map[string]int64{}
	}
	for nonce, expires := range s.used {
		if expires <= now.Unix() {
			delete(s.used, nonce)
		}
	}
	if _, spent := s.used[claims.Nonce]; spent {
		return false
	}
	s.used[claims.Nonce] = claims.Expires
	return true
}
