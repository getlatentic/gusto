package record_test

import (
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/signing"
)

// Every record here has a manifest that claims "whoever-it-claims". The claim
// must never come back as the signer, whatever the keyring can or cannot say.
func TestSignedByNamesOnlyWhatTheKeyringChecked(t *testing.T) {
	ring := t.TempDir()
	acme := keyIn(t, ring, "acme")
	stranger := keyIn(t, t.TempDir(), "stranger")
	keyring := keyringOf(t, ring)
	for name, tc := range map[string]struct {
		signed  bool
		key     signing.Key
		keyring signing.Keyring
		want    string
	}{
		"unsigned":              {keyring: keyring, want: record.Unsigned},
		"signed, no keyring":    {signed: true, key: acme, keyring: signing.Keyring{}, want: record.NoKeyring},
		"a key not in the ring": {signed: true, key: stranger, keyring: keyring, want: stranger.ID() + " is not in the keyring"},
		"a key in the ring":     {signed: true, key: acme, keyring: keyring, want: "acme"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := recorded(t, "yes")
			if tc.signed {
				dir = signed(t, "yes", tc.key)
			}
			got := record.SignedBy(dir, tc.keyring)
			if !strings.Contains(got, tc.want) || strings.Contains(got, "whoever-it-claims") {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSignedBySaysASignatureThatDoesNotHoldIsRefused(t *testing.T) {
	ring := t.TempDir()
	dir := signed(t, "yes", keyIn(t, ring, "acme"))
	text, err := os.ReadFile(dir.OutFile("manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir.OutFile("manifest.json"), strings.Replace(string(text), "whoever-it-claims", "acme", 1))
	if got := record.SignedBy(dir, keyringOf(t, ring)); got != record.Refused {
		t.Fatalf("want %q, got %q", record.Refused, got)
	}
}

func TestSignedBySaysAnUnreadableSignatureIsUnreadable(t *testing.T) {
	dir := recorded(t, "yes")
	writeFile(t, dir.OutFile(record.SignatureFile), "not json")
	if got := record.SignedBy(dir, signing.Keyring{}); !strings.HasPrefix(got, "manifest.sig cannot be read") {
		t.Fatalf("want the signature named, got %q", got)
	}
}
