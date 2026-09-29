// Package entry is how a hosted organisation's server knows who a person is
// without a sign-in page of its own. The front door signs a person in, and then
// hands their browser a token, good for a minute and for one use, that the
// organisation's server checks against a key only the two of them share.
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

// Claims are what the front door says: who this is, what they may do in the
// organisation, which organisation it is, and until when the word is good.
type Claims struct {
	Email   string `json:"e"`
	Role    string `json:"r"`
	Org     string `json:"o"`
	Expires int64  `json:"x"`
	Nonce   string `json:"n"`
}

// Life is how long a token works. A browser follows it at once.
const Life = time.Minute

// ErrInvalid is every reason a token does not work, said once.
var ErrInvalid = errors.New("that sign-in link has been used or has expired")

// Mint is a token for a person to enter an organisation with.
func Mint(key []byte, org, email, role string, now time.Time) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	text, err := json.Marshal(Claims{Email: email, Role: role, Org: org, Expires: now.Add(Life).Unix(), Nonce: base64.RawURLEncoding.EncodeToString(nonce)})
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
// organisation, and has not run out. It does not say the token is unused: that
// is Seen's.
func Verify(key []byte, org, token string, now time.Time) (Claims, error) {
	payload, signature, found := strings.Cut(token, ".")
	if !found || !hmac.Equal([]byte(signature), []byte(sign(key, payload))) {
		return Claims{}, ErrInvalid
	}
	text, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Claims{}, ErrInvalid
	}
	var claims Claims
	if err := json.Unmarshal(text, &claims); err != nil || claims.Org != org || claims.Email == "" || claims.Nonce == "" || !now.Before(time.Unix(claims.Expires, 0)) {
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
