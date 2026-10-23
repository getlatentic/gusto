package evidence

import (
	"fmt"

	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
)

// The words a record's signer is given where no keyring checked a name. They
// live here, beside Attest, so that every surface naming a signer says the same
// thing about a record it cannot vouch for.
const (
	// Unsigned is a record with no manifest.sig beside its manifest.
	Unsigned = "unsigned"
	// NoKeyring is a signed record read where GUSTO_KEYRING names no keys, so
	// nothing here can say whose key made the signature.
	NoKeyring = "signed, not checked: no keyring"
	// Refused is a signature from a key the keyring holds that does not hold
	// for the manifest beside it.
	Refused = "signed, and the signature does not hold"
)

// SignedBy is who signed the record, in the words every surface uses. It gives
// a name only where the keyring checked the signature against its own copy of
// the key, so the manifest's signer field, which is a claim, never appears.
// Every other state is said in words rather than refused, because a page or a
// pull request describing the record still has to say something about it.
func SignedBy(dir job.Dir, keyring signing.Keyring) string {
	signature, manifest, found, err := sealed(dir)
	switch {
	case err != nil:
		return fmt.Sprintf("%s cannot be read: %v", SignatureFile, err)
	case !found:
		return Unsigned
	case len(keyring) == 0:
		return NoKeyring
	}
	if _, held := keyring[signature.Key]; !held {
		return notInKeyring(signature.Key)
	}
	named, err := keyring.Signer(signature, manifest)
	if err != nil {
		return Refused
	}
	return named.Name
}

// notInKeyring names the key rather than a person: the key is what the files
// prove, and whose it is is what the keyring could not say.
func notInKeyring(key string) string {
	return fmt.Sprintf("signed, not checked: key %s is not in the keyring", key)
}
