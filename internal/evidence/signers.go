package evidence

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/signing"
)

// Signers is what every signed record under a directory says about its key,
// read against a keyring: the tally an operator wants before retiring a key,
// after restoring one, and when a record stops verifying.
type Signers struct {
	// Signed is the records each named signer stands behind.
	Signed map[string]int
	// Unsigned is the records nobody signed.
	Unsigned int
	// Unknown is the records signed by a key the keyring does not hold, by key
	// id. They still hold together; nobody can say whose they are.
	Unknown map[string]int
	// Broken is the tasks whose signature does not hold for their manifest, or
	// cannot be read.
	Broken []string
}

// Sound is whether every record that was signed can be attributed.
func (s Signers) Sound() bool { return len(s.Unknown) == 0 && len(s.Broken) == 0 }

// SignersUnder reads every task directory in root that has a record.
func SignersUnder(root string, keyring signing.Keyring) (Signers, error) {
	found := Signers{Signed: map[string]int{}, Unknown: map[string]int{}}
	listing, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return found, nil
	}
	if err != nil {
		return found, err
	}
	for _, entry := range listing {
		if !entry.IsDir() {
			continue
		}
		dir := job.Dir(filepath.Join(root, entry.Name()))
		if _, err := os.Stat(dir.OutFile("manifest.json")); err != nil {
			continue
		}
		attestation, err := Attest(dir, keyring)
		var unknown signing.UnknownKeyError
		switch {
		case errors.As(err, &unknown):
			found.Unknown[unknown.ID]++
		case err != nil:
			found.Broken = append(found.Broken, entry.Name())
		case !attestation.Signed:
			found.Unsigned++
		default:
			found.Signed[attestation.Signer]++
		}
	}
	sort.Strings(found.Broken)
	return found, nil
}
