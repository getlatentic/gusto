package runtoken

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

// proofClaims are what a DPoP proof says: which request it is for, when, and, for a call that shows a token, which token.
type proofClaims struct {
	JTI string `json:"jti"`
	HTM string `json:"htm"`
	HTU string `json:"htu"`
	IAT int64  `json:"iat"`
	ATH string `json:"ath,omitempty"`
}

// proofWindow is how far a proof's time may be from the server's. A proof is made for one request, just before it is sent.
const proofWindow = 60 * time.Second

// Prove makes the DPoP proof for one request, with the run's key. token is the access token the request shows, or empty for
// the exchange that gets one.
func Prove(key ed25519.PrivateKey, method, url, token string, now time.Time) (string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	claims := proofClaims{JTI: b64.EncodeToString(id), HTM: method, HTU: url, IAT: now.Unix()}
	if token != "" {
		claims.ATH = tokenHash(token)
	}
	k := keyOf(key.Public().(ed25519.PublicKey))
	return sign(header{Typ: "dpop+jwt", JWK: &k}, claims, key)
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return b64.EncodeToString(sum[:])
}

// replays remembers the proofs seen, until they are too old to be accepted anyway, so none is accepted twice.
type replays struct {
	mu   sync.Mutex
	seen map[string]time.Time
	most int
}

// first says whether the proof id is new, and remembers it. Past most remembered, it refuses rather than forgets one that is
// still fresh: a proof is good for proofWindow either side of now, so most bounds the calls a server takes in that time.
func (r *replays) first(id string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, seen := r.seen[id]; seen {
		return false
	}
	if len(r.seen) >= r.most {
		for each, until := range r.seen {
			if now.After(until) {
				delete(r.seen, each)
			}
		}
		if len(r.seen) >= r.most {
			return false
		}
	}
	r.seen[id] = now.Add(2 * proofWindow)
	return true
}

// checkProof checks a DPoP proof for a request and says the thumbprint of the key that signed it. token is the access token
// the request shows, which the proof has to name, or empty for an exchange.
func (r *replays) checkProof(proof, method, url, token string, now time.Time) (string, error) {
	var claims proofClaims
	h, signed, signature, err := parse(proof, &claims)
	if err != nil {
		return "", err
	}
	if h.Typ != "dpop+jwt" || h.JWK == nil {
		return "", errors.New("the proof is not a DPoP proof with its key")
	}
	public, err := h.JWK.public()
	if err != nil {
		return "", err
	}
	if err := verify(public, signed, signature); err != nil {
		return "", err
	}
	at := time.Unix(claims.IAT, 0)
	switch {
	case claims.HTM != method || claims.HTU != url:
		return "", errors.New("the proof is for another request")
	case at.Before(now.Add(-proofWindow)) || at.After(now.Add(proofWindow)):
		return "", errors.New("the proof is too old or from the future")
	case token != "" && claims.ATH != tokenHash(token):
		return "", errors.New("the proof is for another token")
	case claims.JTI == "" || !r.first(claims.JTI, now):
		return "", errors.New("the proof was used before")
	}
	return h.JWK.thumbprint(), nil
}
