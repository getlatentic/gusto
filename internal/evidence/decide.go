package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/internal/atomicfile"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
)

// PendingFile is a decision on its way under the record's signature, beside out/ so the manifest does not hash it: the
// decision, the manifest and the signature as they will be, kept until all three are in place.
const PendingFile = "seal.pending"

type pending struct {
	Decision  []byte `json:"decision"`
	Manifest  []byte `json:"manifest"`
	Signature []byte `json:"signature,omitempty"`
}

func pendingFile(dir job.Dir) string { return filepath.Join(dir.Path(), PendingFile) }

// Decide keeps what was done with what a room's turn changed, and puts it under the record's signature: the manifest is made
// again with the decision and signed with the key configured. All three are made and kept in PendingFile before any is put in
// place, so a crash part way leaves what Finish completes, and a key that cannot sign changes nothing. A record that no longer
// checks is not decided, since making its manifest again would vouch for what changed, nor is a signed one with no key to
// sign it again.
func Decide(dir job.Dir, decision Decision) error {
	signer, signs, err := signing.Configured()
	if err != nil {
		return err
	}
	if err := decidable(dir, signs); err != nil {
		return err
	}
	next, err := prepare(dir, decision, signer, signs)
	if err != nil {
		return err
	}
	text, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := atomicfile.WriteFile(pendingFile(dir), text, 0o644); err != nil {
		return err
	}
	return put(dir, next)
}

// decidable says why a record may not be decided, or nothing when it may.
func decidable(dir job.Dir, signs bool) error {
	wrong, err := Check(dir)
	if err != nil {
		return err
	}
	if len(wrong) > 0 {
		return fmt.Errorf("the record does not check, so it is not made again: %s", strings.Join(wrong, "; "))
	}
	if _, err := os.Stat(dir.OutFile(SignatureFile)); err == nil && !signs {
		return errors.New("the record is signed, and no key is configured to sign it again")
	}
	return nil
}

// prepare makes the decision, the manifest that hashes it with the rest of out/, and the manifest's signature.
func prepare(dir job.Dir, decision Decision, signer signing.Signer, signs bool) (pending, error) {
	text, err := json.MarshalIndent(decision, "", "  ")
	if err != nil {
		return pending{}, err
	}
	next := pending{Decision: append(text, '\n')}
	manifest, err := Read(dir)
	if err != nil {
		return pending{}, err
	}
	if manifest.Job, manifest.Files, err = Of(dir); err != nil {
		return pending{}, err
	}
	manifest.Files = withFile(manifest.Files, DecisionFile, next.Decision)
	if next.Manifest, err = encode(manifest); err != nil || !signs {
		return next, err
	}
	signature, err := signing.Sign(signer, next.Manifest)
	if err != nil {
		return pending{}, err
	}
	next.Signature, err = signature.Encode()
	return next, err
}

// withFile is the entries with the named file's as it will be, in name order.
func withFile(files []Entry, name string, text []byte) []Entry {
	sum := sha256.Sum256(text)
	entry := Entry{Name: name, SHA256: hex.EncodeToString(sum[:])}
	kept := make([]Entry, 0, len(files)+1)
	for _, each := range files {
		if each.Name < name {
			kept = append(kept, each)
		}
	}
	kept = append(kept, entry)
	for _, each := range files {
		if each.Name > name {
			kept = append(kept, each)
		}
	}
	return kept
}

// put puts a pending decision in place: the decision, the manifest, then its signature, which a manifest with no signature
// takes away. Each step may be done again.
func put(dir job.Dir, next pending) error {
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		return err
	}
	if err := atomicfile.WriteFile(dir.OutFile(DecisionFile), next.Decision, 0o644); err != nil {
		return err
	}
	if err := atomicfile.WriteFile(dir.OutFile("manifest.json"), next.Manifest, 0o644); err != nil {
		return err
	}
	if next.Signature == nil {
		if err := os.Remove(dir.OutFile(SignatureFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else if err := atomicfile.WriteFile(dir.OutFile(SignatureFile), next.Signature, 0o644); err != nil {
		return err
	}
	return os.Remove(pendingFile(dir))
}

// Finish puts in place a decision a crash left pending, as a server does for the turns that were running when the one before it
// stopped. A pending signature that does not hold for its manifest was not made by Decide, and is refused.
func Finish(dir job.Dir) error {
	text, err := os.ReadFile(pendingFile(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var next pending
	if err := json.Unmarshal(text, &next); err != nil || len(next.Manifest) == 0 || len(next.Decision) == 0 {
		return fmt.Errorf("%s is not a pending decision", pendingFile(dir))
	}
	if next.Signature != nil {
		signature, err := signing.Decode(PendingFile, next.Signature)
		if err != nil {
			return err
		}
		if err := signature.Intact(next.Manifest); err != nil {
			return fmt.Errorf("%s: %w", pendingFile(dir), err)
		}
	}
	return put(dir, next)
}
