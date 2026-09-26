package evidence_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/signing"
)

// recorded is a finished job directory: a spec, one output and a manifest that
// claims a signer the keyring may never have heard of.
func recorded(t *testing.T, answer string) job.Dir {
	t.Helper()
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir.Spec(), `{"brief": "b", "repo": "r", "gate": ["true"], "harness": "h"}`)
	writeFile(t, dir.OutFile("answer.txt"), answer)
	spec, files, err := evidence.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{Job: spec, Files: files, Signer: "whoever-it-claims"}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func signed(t *testing.T, answer string, key signing.Key) job.Dir {
	t.Helper()
	dir := recorded(t, answer)
	if err := evidence.Seal(dir, key); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func keyIn(t *testing.T, ring, name string) signing.Key {
	t.Helper()
	key, err := signing.Generate(ring, name)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func keyringOf(t *testing.T, ring string) signing.Keyring {
	t.Helper()
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	return keyring
}

func complaints(t *testing.T, dir job.Dir) []string {
	t.Helper()
	wrong, err := evidence.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	return wrong
}

func TestASignedRecordVerifiesAndNamesTheSignerFromTheKeyring(t *testing.T) {
	ring := t.TempDir()
	key := keyIn(t, ring, "acme")
	dir := signed(t, "yes", key)
	if wrong := complaints(t, dir); len(wrong) != 0 {
		t.Fatalf("a signed record should check clean, got %v", wrong)
	}
	attested, err := evidence.Attest(dir, keyringOf(t, ring))
	if err != nil {
		t.Fatal(err)
	}
	if !attested.Signed || attested.Signer != "acme" || attested.Key != key.ID() {
		t.Fatalf("want acme's key, got %+v", attested)
	}
	if said := attested.String(); !strings.Contains(said, "signed by acme") || strings.Contains(said, "whoever-it-claims") {
		t.Fatalf("the signer comes from the keyring, got %q", said)
	}
}

func TestAnUnsignedRecordSaysSoAndNeverReadsAsSigned(t *testing.T) {
	dir := recorded(t, "yes")
	if wrong := complaints(t, dir); len(wrong) != 0 {
		t.Fatalf("an unsigned record still checks clean, got %v", wrong)
	}
	attested, err := evidence.Attest(dir, keyringOf(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if attested.Signed || !strings.HasPrefix(attested.String(), "unsigned") {
		t.Fatalf("want unsigned, got %+v", attested)
	}
}

func TestAChangedManifestByteBreaksTheSignature(t *testing.T) {
	ring := t.TempDir()
	dir := signed(t, "yes", keyIn(t, ring, "acme"))
	text, err := os.ReadFile(dir.OutFile("manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir.OutFile("manifest.json"), strings.Replace(string(text), "whoever-it-claims", "whoever-it-clainz", 1))
	if wrong := complaints(t, dir); len(wrong) != 1 || wrong[0] != "manifest.sig does not match manifest.json" {
		t.Fatalf("want the signature named, got %v", wrong)
	}
	if _, err := evidence.Attest(dir, keyringOf(t, ring)); err == nil || !strings.Contains(err.Error(), "out/manifest.sig") {
		t.Fatalf("want the signature refused, got %v", err)
	}
}

func TestAChangedOutputIsCaughtByItsHashUnderASignature(t *testing.T) {
	dir := signed(t, "yes", keyIn(t, t.TempDir(), "acme"))
	writeFile(t, dir.OutFile("answer.txt"), "no")
	if wrong := complaints(t, dir); len(wrong) != 1 || wrong[0] != "answer.txt changed" {
		t.Fatalf("want answer.txt named, got %v", wrong)
	}
}

func TestASignatureSwappedInFromAnotherRecordIsRefused(t *testing.T) {
	ring := t.TempDir()
	key := keyIn(t, ring, "acme")
	dir := signed(t, "yes", key)
	other := signed(t, "no", key)
	swapped, err := os.ReadFile(other.OutFile(evidence.SignatureFile))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir.OutFile(evidence.SignatureFile), string(swapped))
	if wrong := complaints(t, dir); len(wrong) != 1 || wrong[0] != "manifest.sig does not match manifest.json" {
		t.Fatalf("want the swapped signature named, got %v", wrong)
	}
	if _, err := evidence.Attest(dir, keyringOf(t, ring)); err == nil {
		t.Fatal("want the swapped signature refused")
	}
}

func TestASignatureFromAKeyNotInTheKeyringIsRefusedByName(t *testing.T) {
	ring := t.TempDir()
	keyIn(t, ring, "acme")
	stranger := keyIn(t, t.TempDir(), "stranger")
	dir := signed(t, "yes", stranger)
	if wrong := complaints(t, dir); len(wrong) != 0 {
		t.Fatalf("the signature holds for its own key, so the check is clean: %v", wrong)
	}
	_, err := evidence.Attest(dir, keyringOf(t, ring))
	if err == nil || !strings.Contains(err.Error(), stranger.ID()) || !strings.Contains(err.Error(), "not in the keyring") {
		t.Fatalf("want the stranger's key named, got %v", err)
	}
}

// A key is rotated by adding the new public key and keeping the old one: the
// old private key is gone, and what it signed still verifies.
func TestARotatedKeyStillVerifiesTheRecordsItSigned(t *testing.T) {
	ring := t.TempDir()
	before := signed(t, "yes", keyIn(t, ring, "acme-2025"))
	if err := os.Remove(filepath.Join(ring, "acme-2025.key")); err != nil {
		t.Fatal(err)
	}
	after := signed(t, "yes", keyIn(t, ring, "acme-2026"))
	keyring := keyringOf(t, ring)
	for dir, want := range map[job.Dir]string{before: "acme-2025", after: "acme-2026"} {
		attested, err := evidence.Attest(dir, keyring)
		if err != nil || attested.Signer != want {
			t.Fatalf("want %s, got %+v: %v", want, attested, err)
		}
	}
}

func TestAnUnreadableSignatureIsNamed(t *testing.T) {
	dir := recorded(t, "yes")
	writeFile(t, dir.OutFile(evidence.SignatureFile), "not json")
	if wrong := complaints(t, dir); len(wrong) != 1 || !strings.HasPrefix(wrong[0], "manifest.sig cannot be read") {
		t.Fatalf("want the signature named, got %v", wrong)
	}
}

func TestRewritingAManifestDropsTheSignatureItNoLongerHas(t *testing.T) {
	dir := signed(t, "yes", keyIn(t, t.TempDir(), "acme"))
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir.OutFile(evidence.SignatureFile)); !os.IsNotExist(err) {
		t.Fatalf("want the stale signature gone, got %v", err)
	}
}

func TestSealAsConfiguredSignsWithTheNamedKeyOrNotAtAll(t *testing.T) {
	unkeyed := recorded(t, "yes")
	t.Setenv(signing.KeyEnv, "")
	if err := evidence.SealAsConfigured(unkeyed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unkeyed.OutFile(evidence.SignatureFile)); !os.IsNotExist(err) {
		t.Fatalf("no key, no signature: %v", err)
	}
	ring := t.TempDir()
	keyIn(t, ring, "acme")
	t.Setenv(signing.KeyEnv, filepath.Join(ring, "acme.key"))
	keyed := recorded(t, "yes")
	if err := evidence.SealAsConfigured(keyed); err != nil {
		t.Fatal(err)
	}
	if attested, err := evidence.Attest(keyed, keyringOf(t, ring)); err != nil || attested.Signer != "acme" {
		t.Fatalf("want acme, got %+v: %v", attested, err)
	}
	if err := evidence.SealAsConfigured(job.Dir(t.TempDir())); err != nil {
		t.Fatalf("a directory with no manifest has nothing to sign: %v", err)
	}
}
