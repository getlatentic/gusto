package audit

import (
	"fmt"

	"github.com/getlatentic/gusto/internal/evidence"
)

// asWritten is the derived summary as the version the export was written at
// says it. Version 1 carried the manifest's signer claim as it stood, so a
// version 1 export is held to exactly that and still verifies; it vouches for
// no signer, and a reader of version 1 is told so by the schema.
func (r Record) asWritten(version int, manifest evidence.Manifest) Record {
	if version != 1 {
		return r
	}
	r.Version = 1
	r.Evidence.Signer = manifest.Signer
	return r
}

// signerDisagreement is the one comparison that rests on a keyring rather than
// on files. The exporter's keyring and the verifier's may differ, and when they
// do the verifier is shown both answers rather than told which to believe.
func signerDisagreement(written, derived string) []string {
	if written == derived {
		return nil
	}
	return []string{fmt.Sprintf("%s: evidence.signer says %q, and this keyring says %q", SummaryFile, written, derived)}
}
