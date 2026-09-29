package signing

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/getlatentic/haas/internal/job"
)

// Version is the format of a signature file this code writes and the newest
// it reads. The schema that describes it is schema/signature/v1.json.
const Version = 1

// ErrMismatch is a signature that does not hold for the bytes it sits beside.
var ErrMismatch = errors.New("the signature does not match")

// Signature is a detached signature over a file's exact bytes. It carries the
// public key it was made with, so anybody can tell the bytes changed after
// signing without a keyring; who the key belongs to is only the keyring's word.
type Signature struct {
	Version   int    `json:"version"`
	Scheme    string `json:"scheme"`
	Key       string `json:"key"`
	PublicKey string `json:"publicKey"`
	Value     string `json:"signature"`
}

// Sign signs the message with the signer, in the scheme it names.
func Sign(signer Signer, message []byte) (Signature, error) {
	value, err := signer.Sign(message)
	if err != nil {
		return Signature{}, err
	}
	return Signature{
		Version:   Version,
		Scheme:    signer.Scheme(),
		Key:       signer.ID(),
		PublicKey: base64.StdEncoding.EncodeToString(signer.Public()),
		Value:     base64.StdEncoding.EncodeToString(value),
	}, nil
}

// Encode is the signature as the file holds it.
func (s Signature) Encode() ([]byte, error) {
	text, err := json.MarshalIndent(s, "", "  ")
	return append(text, '\n'), err
}

// Decode reads a signature file, refusing a version it does not know for the
// reasons job.Readable gives.
func Decode(name string, text []byte) (Signature, error) {
	var signature Signature
	if err := json.Unmarshal(text, &signature); err != nil {
		return Signature{}, fmt.Errorf("%s: %w", name, err)
	}
	if err := job.Readable(name, signature.Version, Version); err != nil {
		return Signature{}, err
	}
	if signature.Scheme != Scheme && signature.Scheme != SchemeDigest {
		return Signature{}, fmt.Errorf("%s uses the scheme %q, and this build knows %q and %q", name, signature.Scheme, Scheme, SchemeDigest)
	}
	return signature, nil
}

// Intact checks the signature against the key it carries: that the key is the
// one its id names, and that it signed these bytes. It says nothing about who
// holds the key.
func (s Signature) Intact(message []byte) error {
	public, err := decodeKey(s.PublicKey)
	if err != nil {
		return err
	}
	if IDOf(public) != s.Key {
		return fmt.Errorf("the key it carries is %s, and it names %s", IDOf(public), s.Key)
	}
	return s.madeBy(public, message)
}

func (s Signature) madeBy(public ed25519.PublicKey, message []byte) error {
	value, err := base64.StdEncoding.DecodeString(s.Value)
	if err != nil {
		return fmt.Errorf("the signature is not base64: %w", err)
	}
	if !verify(s.Scheme, public, message, value) {
		return ErrMismatch
	}
	return nil
}

func decodeKey(text string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, fmt.Errorf("the public key is not base64: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("the public key is %d bytes, and an Ed25519 key is %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}
