package signing

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// KeyringEnv names the directory of public keys a verifier trusts.
const KeyringEnv = "GUSTO_KEYRING"

// Named is a public key and the name its keyring file gives it.
type Named struct {
	Name   string
	Public ed25519.PublicKey
}

// Keyring holds the public keys a verifier trusts, by id. A retired key stays
// in it after its private half is destroyed, which is what keeps the records
// that key signed verifiable.
type Keyring map[string]Named

// ConfiguredKeyring is the keyring GUSTO_KEYRING names, or an empty one when
// the variable is unset, in which every signature is from a key not in it.
func ConfiguredKeyring() (Keyring, error) {
	dir := os.Getenv(KeyringEnv)
	if dir == "" {
		return Keyring{}, nil
	}
	return OpenKeyring(dir)
}

// OpenKeyring reads every <name>.pub in a directory. The name is the signer's,
// so two files holding one key would give a record two signers and are refused.
func OpenKeyring(dir string) (Keyring, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.pub"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	ring := Keyring{}
	for _, path := range paths {
		if err := ring.add(path); err != nil {
			return nil, err
		}
	}
	return ring, nil
}

func (k Keyring) add(path string) error {
	public, err := ReadPublic(path)
	if err != nil {
		return err
	}
	name := strings.TrimSuffix(filepath.Base(path), ".pub")
	id := IDOf(public)
	if held, found := k[id]; found {
		return fmt.Errorf("the keyring holds key %s twice, as %s and %s", id, held.Name, name)
	}
	k[id] = Named{Name: name, Public: public}
	return nil
}

// UnknownKeyError is a signature from a key the keyring does not hold. It is
// not a forgery: it is what a record looks like once its key has been left out
// of the keyring, so a check that lists these is listing what has stopped being
// verifiable, and what a restored key or public file would bring back.
type UnknownKeyError struct{ ID string }

func (e UnknownKeyError) Error() string {
	return fmt.Sprintf("it was made by key %s, which is not in the keyring", e.ID)
}

// Signer names who signed the message: the keyring's name for the key, checked
// against the keyring's own copy of it rather than the one the signature
// carries. A key the keyring does not hold is refused by its id.
func (k Keyring) Signer(signature Signature, message []byte) (Named, error) {
	held, found := k[signature.Key]
	if !found {
		return Named{}, UnknownKeyError{ID: signature.Key}
	}
	if err := signature.madeBy(held.Public, message); err != nil {
		return Named{}, err
	}
	return held, nil
}

// Named finds a key by the name its keyring file gives it.
func (k Keyring) Named(name string) (Named, bool) {
	for _, held := range k {
		if held.Name == name {
			return held, true
		}
	}
	return Named{}, false
}

// PEM is the key as its <name>.pub file holds it, for a verifier to keep.
func (n Named) PEM() ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(n.Public)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}
