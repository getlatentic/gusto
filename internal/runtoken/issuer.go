package runtoken

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"time"
)

// Run is what a run's token says of it besides the run itself: the person it acts for, the agent, and the room it works in.
type Run struct {
	Person, Agent, Room string
}

// detail is one entry of authorization_details (RFC 9396): what the token may be used for, and where.
type detail struct {
	Type    string   `json:"type"`
	Room    string   `json:"room,omitempty"`
	Actions []string `json:"actions"`
}

type confirmation struct {
	JKT string `json:"jkt"`
}

// access is a run token's claims, as RFC 9068 names them, with the run and its key's thumbprint.
type access struct {
	Issuer   string       `json:"iss"`
	Subject  string       `json:"sub,omitempty"`
	Audience string       `json:"aud"`
	ClientID string       `json:"client_id"`
	Run      string       `json:"run"`
	ID       string       `json:"jti"`
	Issued   int64        `json:"iat"`
	Expires  int64        `json:"exp"`
	Cnf      confirmation `json:"cnf"`
	Details  []detail     `json:"authorization_details"`
}

// Audience is who a run token is for: gusto's proxy, which is where a run spends and nowhere else.
const Audience = "gusto-proxy"

const issuer = "gusto"

// Issuer mints and checks run tokens. Its key is made when the server starts and kept nowhere, so a token outlives neither the
// run nor the server.
type Issuer struct {
	key      ed25519.PrivateKey
	ttl      time.Duration
	now      func() time.Time
	proofs   *replays
	describe func(run string) Run
}

// NewIssuer is an issuer of tokens good for ttl. describe says who and what a run is for, from the server's own records.
func NewIssuer(ttl time.Duration, describe func(run string) Run) (*Issuer, error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Issuer{key: key, ttl: ttl, now: time.Now, describe: describe, proofs: &replays{seen: map[string]time.Time{}, most: 1 << 20}}, nil
}

// Exchange trades a run's bootstrap secret, which redeem takes once, or a token of the run that is still good, for a token bound
// to the key that signed the proof. It is an OAuth token exchange (RFC 8693) with the proof of RFC 9449.
func (i *Issuer) Exchange(subject, proof, method, url string, redeem func(secret string) (run string, ok bool)) (string, error) {
	if strings.Count(subject, ".") == 2 {
		return i.refresh(subject, proof, method, url)
	}
	thumbprint, err := i.proofs.checkProof(proof, method, url, "", i.now())
	if err != nil {
		return "", err
	}
	run, ok := redeem(subject)
	if !ok {
		return "", errors.New("that secret was used, or never given")
	}
	return i.mint(run, thumbprint)
}

// refresh is a token of a run that is still good traded for a new one, bound to the same key.
func (i *Issuer) refresh(token, proof, method, url string) (string, error) {
	held, thumbprint, err := i.check(token, proof, method, url)
	if err != nil {
		return "", err
	}
	if thumbprint != held.Cnf.JKT {
		return "", errors.New("the proof is not by the token's key")
	}
	return i.mint(held.Run, thumbprint)
}

func (i *Issuer) mint(run, thumbprint string) (string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	who, now := i.describe(run), i.now()
	actions := []string{"model", "tools"}
	return sign(header{Typ: "at+jwt"}, access{
		Issuer: issuer, Subject: who.Person, Audience: Audience, ClientID: firstOf(who.Agent, "gusto-run"), Run: run,
		ID: b64.EncodeToString(id), Issued: now.Unix(), Expires: now.Add(i.ttl).Unix(), Cnf: confirmation{JKT: thumbprint},
		Details: []detail{{Type: "gusto_run", Room: who.Room, Actions: actions}},
	}, i.key)
}

// Check is the run a request stands for: its token is this server's, for the proxy, not expired, and the proof is fresh, for
// this request and signed by the key the token is bound to.
func (i *Issuer) Check(token, proof, method, url string) (string, error) {
	held, thumbprint, err := i.check(token, proof, method, url)
	if err != nil {
		return "", err
	}
	if thumbprint != held.Cnf.JKT {
		return "", errors.New("the proof is not by the token's key")
	}
	return held.Run, nil
}

func (i *Issuer) check(token, proof, method, url string) (access, string, error) {
	var held access
	h, signed, signature, err := parse(token, &held)
	if err != nil {
		return access{}, "", err
	}
	now := i.now()
	switch {
	case h.Typ != "at+jwt":
		return access{}, "", errors.New("that is not an access token")
	case verify(i.key.Public().(ed25519.PublicKey), signed, signature) != nil:
		return access{}, "", errors.New("the token is not this server's")
	case held.Issuer != issuer || held.Audience != Audience || held.Run == "":
		return access{}, "", errors.New("the token is not for this proxy")
	case now.Unix() >= held.Expires:
		return access{}, "", errors.New("the token has expired")
	}
	thumbprint, err := i.proofs.checkProof(proof, method, url, token, now)
	return held, thumbprint, err
}

func firstOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
