package web

import (
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/pkg/job"
)

// proof is what the page says about a record after checking it the way
// `gusto verify` does: whether its files still match, and who signed it.
type proof struct {
	Files  int
	Wrong  []string
	Signed bool
	Signer string
	// Unchecked is why a signature whose files match could not be tied to a
	// signer: its key is not one this server holds.
	Unchecked string
	// Key is where the signer's public key is served, the one file a verifier
	// needs from this server, and needs once.
	Key     string
	Bundle  string
	Command string
}

// Holds is a record whose files match and whose signature, if any, checks.
func (p proof) Holds() bool { return len(p.Wrong) == 0 }

func (s *Server) proofOf(id string, dir job.Dir, manifest *evidence.Manifest) *proof {
	if manifest == nil {
		return nil
	}
	shown := &proof{Files: len(manifest.Files), Bundle: "/jobs/" + id + "/bundle"}
	wrong, err := evidence.Check(dir)
	if err != nil {
		wrong = append(wrong, err.Error())
	}
	shown.Wrong = wrong
	attested, err := evidence.Attest(dir, s.keyring)
	if err != nil && shown.Holds() {
		shown.Unchecked = evidence.SignedBy(dir, s.keyring)
	}
	unpack := "tar -xzf " + shellWord(id+".tar.gz") + " && "
	if !attested.Signed {
		shown.Command = unpack + "gusto verify " + shellWord(id)
		return shown
	}
	shown.Signed, shown.Signer, shown.Key = true, attested.Signer, "/keys/"+attested.Signer+".pub"
	shown.Command = unpack + "GUSTO_KEYRING=. gusto verify " + shellWord(id) + " -signed"
	return shown
}

// key serves a signer's public key by the name the keyring gives it. It is
// public: a verifier holds it before trusting anything this server says.
func (s *Server) key(w http.ResponseWriter, r *http.Request) {
	held, found := s.keyring.Named(strings.TrimSuffix(r.PathValue("file"), ".pub"))
	if !found {
		http.NotFound(w, r)
		return
	}
	text, err := held.PEM()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="`+held.Name+`.pub"`)
	w.Write(text)
}
