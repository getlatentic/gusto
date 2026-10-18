// Package signing is the key a runner signs a record with, and the keyring a
// verifier names the signer from.
//
// The scheme is Ed25519 from the standard library over the exact bytes of a
// file, so the signature needs no canonical form and any Ed25519 tool can
// check it. Keys are PEM files: PKCS#8 for a private key, PKIX for a public
// one, the forms openssl reads too.
package signing

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// KeyEnv names the private key a runner signs with: a file on the machine that
// runs gusto, never in a task directory or a container, or a KMS key written
// awskms://<key>.
const KeyEnv = "GUSTO_SIGNING_KEY"

// Key is a private key a runner holds in a file and signs with.
type Key struct {
	private ed25519.PrivateKey
}

func (k Key) Scheme() string { return Scheme }

func (k Key) Sign(message []byte) ([]byte, error) { return ed25519.Sign(k.private, message), nil }

// ID is the id of the key's public half, which a signature names.
func (k Key) ID() string { return IDOf(k.Public()) }

// Public is the half a verifier keeps in a keyring.
func (k Key) Public() ed25519.PublicKey {
	return k.private.Public().(ed25519.PublicKey)
}

// IDOf names a public key by its hash, so the id cannot be chosen apart from
// the key. Sixteen bytes tell keys apart; verification never trusts the id
// alone, because it checks the signature against the keyring's key itself.
func IDOf(public ed25519.PublicKey) string {
	sum := sha256.Sum256(public)
	return "sha256:" + hex.EncodeToString(sum[:16])
}

// Configured is the signer GUSTO_SIGNING_KEY names. Found is false when the
// variable is unset, which leaves a run unsigned rather than failing it; a
// variable naming a file that is not a key is an error, because a runner that
// meant to sign must not quietly stop.
func Configured() (signer Signer, found bool, err error) {
	path := os.Getenv(KeyEnv)
	if path == "" {
		return nil, false, nil
	}
	signer, err = signerFor(context.Background(), path)
	if err != nil {
		return nil, false, err
	}
	return signer, true, nil
}

// ReadKey reads a PEM PKCS#8 Ed25519 private key.
func ReadKey(path string) (Key, error) {
	der, err := pemBlock(path, "PRIVATE KEY")
	if err != nil {
		return Key{}, err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return Key{}, fmt.Errorf("%s: %w", path, err)
	}
	private, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return Key{}, fmt.Errorf("%s is not an Ed25519 key", path)
	}
	return Key{private: private}, nil
}

// ReadPublic reads a PEM PKIX Ed25519 public key.
func ReadPublic(path string) (ed25519.PublicKey, error) {
	der, err := pemBlock(path, "PUBLIC KEY")
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	public, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an Ed25519 public key", path)
	}
	return public, nil
}

// Generate makes a key pair named for its signer and writes it into dir as
// <name>.key, readable by its owner alone, and <name>.pub, which is the file a
// keyring holds and whose name is the signer's. It refuses to replace either,
// because overwriting a key loses every record it could still prove.
func Generate(dir, name string) (Key, error) {
	if err := plainName(name); err != nil {
		return Key{}, err
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Key{}, err
	}
	key := Key{private: private}
	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return Key{}, err
	}
	privatePath := filepath.Join(dir, name+".key")
	if err := writeNew(privatePath, "PRIVATE KEY", privateDER, 0o600); err != nil {
		return Key{}, err
	}
	if err := WritePublic(dir, name, key.Public()); err != nil {
		return Key{}, errors.Join(err, os.Remove(privatePath))
	}
	return key, nil
}

// WritePublic writes the public half of a key into dir as <name>.pub, which is
// the file a keyring holds and whose name is the signer's. A key held in a key
// service has only this file to put in a keyring. It refuses to replace one.
func WritePublic(dir, name string, public ed25519.PublicKey) error {
	if err := plainName(name); err != nil {
		return err
	}
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return err
	}
	return writeNew(filepath.Join(dir, name+".pub"), "PUBLIC KEY", der, 0o644)
}

// plainName keeps a signer's name a file name and nothing more, so it cannot
// write a key somewhere else or hide from a keyring's listing.
func plainName(name string) error {
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%q is not a signer's name: use letters, digits and dashes", name)
	}
	return nil
}

func writeNew(path, kind string, der []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if err := pem.Encode(file, &pem.Block{Type: kind, Bytes: der}); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}

func pemBlock(path, kind string) ([]byte, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(text)
	if block == nil || block.Type != kind {
		return nil, fmt.Errorf("%s holds no PEM %s", path, kind)
	}
	return block.Bytes, nil
}
