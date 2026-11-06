package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/schema"
)

// keyed is a runner with a fresh key and no name, and the keyring holding its public half.
func keyed(t *testing.T, name string) (signing.Identity, signing.Keyring) {
	t.Helper()
	ring := t.TempDir()
	if _, err := signing.Generate(ring, name); err != nil {
		t.Fatal(err)
	}
	identity, err := signing.Open(context.Background(), filepath.Join(ring, name+".key"), "")
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	return identity, keyring
}

func TestARunWithAKeySignsItsManifest(t *testing.T) {
	identity, keyring := keyed(t, "acme")
	dir, _ := ranAs(t, identity)
	if wrong, err := evidence.Check(dir); err != nil || len(wrong) != 0 {
		t.Fatalf("a signed run should check clean, got %v: %v", wrong, err)
	}
	attested, err := evidence.Attest(dir, keyring)
	if err != nil || attested.Signer != "acme" {
		t.Fatalf("want acme, got %+v: %v", attested, err)
	}
	written, err := os.ReadFile(dir.OutFile(evidence.SignatureFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(schema.Signature(), written); err != nil {
		t.Fatalf("manifest.sig does not match its schema:\n%v\n%s", err, written)
	}
}

func TestARunWithNoKeyIsUnsigned(t *testing.T) {
	dir, _ := ranAs(t, signing.Identity{})
	if _, err := os.Stat(dir.OutFile(evidence.SignatureFile)); !os.IsNotExist(err) {
		t.Fatalf("no key, no signature: %v", err)
	}
	attested, err := evidence.Attest(dir, signing.Keyring{})
	if err != nil || attested.Signed {
		t.Fatalf("want unsigned, got %+v: %v", attested, err)
	}
}

// A runner with a key and no name names the key in the manifest, so a signed
// record never calls itself unsigned.
func TestASignedManifestWithNoNameGivesTheKeysID(t *testing.T) {
	identity, keyring := keyed(t, "acme")
	dir, _ := ranAs(t, identity)
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	for id := range keyring {
		if manifest.Signer != id {
			t.Fatalf("want the key's id %s, got %q", id, manifest.Signer)
		}
	}
}
