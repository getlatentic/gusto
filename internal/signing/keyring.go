package signing

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// KeyringEnv names the directory of public keys a verifier trusts.
const KeyringEnv = "HAAS_KEYRING"

// Named is a public key and the name its keyring file gives it.
type Named struct {
	Name   string
	Public ed25519.PublicKey
}

// Keyring holds the public keys a verifier trusts, by id. A retired key stays
// in it after its private half is destroyed, which is what keeps the records
// that key signed verifiable.
type Keyring map[string]Named

// ConfiguredKeyring is the keyring HAAS_KEYRING names, or an empty one when
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

// Signer names who signed the message: the keyring's name for the key, checked
// against the keyring's own copy of it rather than the one the signature
// carries. A key the keyring does not hold is refused by its id.
func (k Keyring) Signer(signature Signature, message []byte) (Named, error) {
	held, found := k[signature.Key]
	if !found {
		return Named{}, fmt.Errorf("it was made by key %s, which is not in the keyring", signature.Key)
	}
	if err := signature.madeBy(held.Public, message); err != nil {
		return Named{}, err
	}
	return held, nil
}
