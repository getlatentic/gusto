package signing

import (
	"crypto/ed25519"
	"crypto/sha512"
)

// Signer is what signs a record. The key a runner holds in a file is one; a key
// a cloud's key service holds and never hands out is the other, and a record
// carries only a public key and a signature, so a verifier cannot tell them
// apart except by the scheme.
type Signer interface {
	// ID is the id of the public key a signature names.
	ID() string
	// Public is the half a verifier keeps in a keyring.
	Public() ed25519.PublicKey
	// Scheme is how Sign signs: Scheme, or SchemeDigest.
	Scheme() string
	// Sign signs the message in that scheme.
	Sign(message []byte) ([]byte, error)
}

// Scheme signs the file's exact bytes. It is plain Ed25519, which any tool
// that reads the key can check.
const Scheme = "ed25519"

// SchemeDigest signs the SHA-512 digest of the file's bytes with plain Ed25519.
// A key service limits the size of what it signs, and a manifest of a large job
// is larger than that limit, so it signs the 64 bytes of the digest instead. It
// is the same key and the same public half. Any tool that checks plain Ed25519
// checks it, given the digest: sha512sum of the file is the message.
const SchemeDigest = "ed25519-sha512"

// verify checks a signature made in the scheme this signature names.
func verify(scheme string, public ed25519.PublicKey, message, value []byte) bool {
	switch scheme {
	case Scheme:
		return ed25519.Verify(public, message, value)
	case SchemeDigest:
		digest := sha512.Sum512(message)
		return ed25519.Verify(public, digest[:], value)
	}
	return false
}
