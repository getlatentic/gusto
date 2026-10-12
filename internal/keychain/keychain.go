// Package keychain keeps the keys a person gives haas on their own computer in the computer's own store of secrets: the
// Keychain on macOS, the Credential Manager on Windows, and the Secret Service on Linux. A key is never written to a file,
// passed as an argument another user could read, or shown.
package keychain

import (
	"errors"
	"fmt"
)

// service is what haas's keys are kept under, each by the provider it is for.
const service = "haas"

// Store is this computer's store of secrets.
type Store interface {
	Keep(provider, key string) error
	// Read is the key kept for a provider, or an error wrapping fs.ErrNotExist when none is.
	Read(provider string) (string, error)
	Forget(provider string) error
}

// probe is kept, read and forgotten when the store is opened, so a store that cannot be used now is known before a person's
// key is given to it.
const probe = "haas-probe"

// Open is this computer's store, when it can be used now.
func Open() (Store, error) {
	store, err := system()
	if err != nil {
		return nil, err
	}
	if err := store.Keep(probe, "a-probe"); err != nil {
		return nil, fmt.Errorf("the keychain could not keep a key: %w", err)
	}
	defer store.Forget(probe)
	if read, err := store.Read(probe); err != nil || read != "a-probe" {
		return nil, errors.Join(errors.New("the keychain did not give back the key it kept"), err)
	}
	return store, nil
}
