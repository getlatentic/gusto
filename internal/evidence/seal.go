package evidence

import (
	"errors"
	"fmt"
	"os"

	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
)

// SignatureFile is the detached signature over manifest.json, beside it in
// out/. It is detached because a signature inside the file it signs would need
// the file in one canonical form; this one covers the bytes as written. The
// manifest cannot hash it, for the reason it cannot hash itself.
const SignatureFile = "manifest.sig"

// Attestation is what the signature says about a record. Signer comes from the
// keyring, never from the manifest's own signer field, which is a claim.
type Attestation struct {
	Signed bool
	Key    string
	Signer string
}

// String says it in a line for a person. Unsigned is said as plainly as signed,
// so a record nobody signed never reads as one somebody did.
func (a Attestation) String() string {
	if !a.Signed {
		return "unsigned: no key stands behind this record"
	}
	return fmt.Sprintf("signed by %s (key %s)", a.Signer, a.Key)
}

// Seal signs manifest.json as it is on disk.
func Seal(dir job.Dir, signer signing.Signer) error {
	manifest, err := os.ReadFile(dir.OutFile("manifest.json"))
	if err != nil {
		return err
	}
	signature, err := signing.Sign(signer, manifest)
	if err != nil {
		return err
	}
	text, err := signature.Encode()
	if err != nil {
		return err
	}
	return os.WriteFile(dir.OutFile(SignatureFile), text, 0o644)
}

// SealAsConfigured signs with the key GUSTO_SIGNING_KEY names, and leaves the
// record unsigned when it names none. A directory with no manifest has nothing
// to sign: a run that died before writing one is reported by its own error.
func SealAsConfigured(dir job.Dir) error {
	signer, found, err := signing.Configured()
	if err != nil || !found {
		return err
	}
	if _, err := os.Stat(dir.OutFile("manifest.json")); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return Seal(dir, signer)
}

// Attest names who signed the record, from the keyring. An unsigned record is
// not an error here: whether one is acceptable is the verifier's to decide.
// A signature that does not hold, or one from a key the keyring does not hold,
// is refused, and the refusal names the file.
func Attest(dir job.Dir, keyring signing.Keyring) (Attestation, error) {
	signature, manifest, found, err := sealed(dir)
	if err != nil || !found {
		return Attestation{}, err
	}
	signer, err := keyring.Signer(signature, manifest)
	if err != nil {
		return Attestation{}, fmt.Errorf("out/%s: %w", SignatureFile, err)
	}
	return Attestation{Signed: true, Key: signature.Key, Signer: signer.Name}, nil
}

// sealComplaints is what Check says about a signature, which needs no keyring:
// whether it still holds for the manifest beside it. An unsigned record draws
// no complaint, because every record made before signing is one.
func sealComplaints(dir job.Dir) []string {
	signature, manifest, found, err := sealed(dir)
	if err != nil {
		return []string{fmt.Sprintf("%s cannot be read: %v", SignatureFile, err)}
	}
	if !found {
		return nil
	}
	if err := signature.Intact(manifest); errors.Is(err, signing.ErrMismatch) {
		return []string{fmt.Sprintf("%s does not match manifest.json", SignatureFile)}
	} else if err != nil {
		return []string{fmt.Sprintf("%s does not hold: %v", SignatureFile, err)}
	}
	return nil
}

// sealed reads the signature and the manifest bytes it signs. Found is false
// when there is no signature.
func sealed(dir job.Dir) (signing.Signature, []byte, bool, error) {
	text, err := os.ReadFile(dir.OutFile(SignatureFile))
	if errors.Is(err, os.ErrNotExist) {
		return signing.Signature{}, nil, false, nil
	}
	if err != nil {
		return signing.Signature{}, nil, false, err
	}
	signature, err := signing.Decode(SignatureFile, text)
	if err != nil {
		return signing.Signature{}, nil, false, err
	}
	manifest, err := os.ReadFile(dir.OutFile("manifest.json"))
	return signature, manifest, err == nil, err
}
