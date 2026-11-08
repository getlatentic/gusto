// Package runtoken is the credential of one run: a short-lived JWT access token (RFC 9068 in shape) that the server signs, bound
// to a key that never leaves the run's sandbox, and shown with a DPoP proof (RFC 9449) on every call. The token names the agent
// in client_id, the person it acts for in sub, the run in run and the room and what may be done there in authorization_details
// (RFC 9396). A token copied out of the sandbox is worth nothing without the key, and nothing at all once the run ends.
//
// Tokens and proofs are signed with Ed25519 (JWS "EdDSA"), from the standard library: no JOSE library is needed for the one
// algorithm used.
package runtoken

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

var b64 = base64.RawURLEncoding

// jwk is an Ed25519 public key as a JSON Web Key.
type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
}

func keyOf(public ed25519.PublicKey) jwk {
	return jwk{Kty: "OKP", Crv: "Ed25519", X: b64.EncodeToString(public)}
}

// thumbprint is the key's JWK thumbprint (RFC 7638): the hash of its required members in lexical order.
func (k jwk) thumbprint() string {
	sum := sha256.Sum256([]byte(`{"crv":"` + k.Crv + `","kty":"` + k.Kty + `","x":"` + k.X + `"}`))
	return b64.EncodeToString(sum[:])
}

func (k jwk) public() (ed25519.PublicKey, error) {
	if k.Kty != "OKP" || k.Crv != "Ed25519" {
		return nil, errors.New("the key is not an Ed25519 key")
	}
	raw, err := b64.DecodeString(k.X)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("the key is not an Ed25519 key")
	}
	return ed25519.PublicKey(raw), nil
}

// header is a JWS protected header. A DPoP proof carries its public key in it.
type header struct {
	Typ string `json:"typ"`
	Alg string `json:"alg"`
	JWK *jwk   `json:"jwk,omitempty"`
}

// sign is the compact JWS of claims under header, signed with key.
func sign(h header, claims any, key ed25519.PrivateKey) (string, error) {
	h.Alg = "EdDSA"
	head, err := json.Marshal(h)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signed := b64.EncodeToString(head) + "." + b64.EncodeToString(body)
	return signed + "." + b64.EncodeToString(ed25519.Sign(key, []byte(signed))), nil
}

// parse splits a compact JWS and reads its header and claims, without checking the signature: verify does that once the key
// is known, which for a DPoP proof is in the header.
func parse(token string, claims any) (header, []byte, []byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(token) > 8<<10 {
		return header{}, nil, nil, errors.New("that is not a signed token")
	}
	var h header
	head, err := b64.DecodeString(parts[0])
	if err != nil || json.Unmarshal(head, &h) != nil || h.Alg != "EdDSA" {
		return header{}, nil, nil, errors.New("that is not a signed token of this server's kind")
	}
	body, err := b64.DecodeString(parts[1])
	if err != nil || json.Unmarshal(body, claims) != nil {
		return header{}, nil, nil, errors.New("the token's claims cannot be read")
	}
	signature, err := b64.DecodeString(parts[2])
	if err != nil {
		return header{}, nil, nil, errors.New("the token's signature cannot be read")
	}
	return h, []byte(parts[0] + "." + parts[1]), signature, nil
}

func verify(public ed25519.PublicKey, signed, signature []byte) error {
	if !ed25519.Verify(public, signed, signature) {
		return errors.New("the signature does not match")
	}
	return nil
}
