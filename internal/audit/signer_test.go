package audit_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/audit"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/signing"
)

// signedRun runs the fixture job signed by a key kept in ring as name. The
// manifest still claims "a-test", which no keyring vouches for.
func signedRun(t *testing.T, ring, name string) job.Dir {
	t.Helper()
	if _, err := signing.Generate(ring, name); err != nil {
		t.Fatal(err)
	}
	t.Setenv(signing.KeyEnv, filepath.Join(ring, name+".key"))
	return ran(t)
}

func keyringAt(t *testing.T, ring string) signing.Keyring {
	t.Helper()
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	return keyring
}

func TestAnExportNamesOnlyTheSignerItsKeyringChecked(t *testing.T) {
	trusted := t.TempDir()
	keyIn := func(name string) {
		if _, err := signing.Generate(trusted, name); err != nil {
			t.Fatal(err)
		}
	}
	keyIn("acme")
	for name, tc := range map[string]struct {
		from    func(t *testing.T) job.Dir
		keyring signing.Keyring
		want    string
	}{
		"unsigned": {
			from: ran, keyring: keyringAt(t, trusted), want: evidence.Unsigned,
		},
		"signed, no keyring": {
			from:    func(t *testing.T) job.Dir { return signedRun(t, t.TempDir(), "stranger") },
			keyring: signing.Keyring{}, want: evidence.NoKeyring,
		},
		"signed by a key not in the keyring": {
			from:    func(t *testing.T) job.Dir { return signedRun(t, t.TempDir(), "stranger") },
			keyring: keyringAt(t, trusted), want: "is not in the keyring",
		},
	} {
		t.Run(name, func(t *testing.T) {
			export := exported(t, tc.from(t), audit.Options{Keyring: tc.keyring})
			signer, _ := at(summary(t, export), "evidence", "signer").(string)
			if !strings.Contains(signer, tc.want) || strings.Contains(signer, "a-test") {
				t.Fatalf("want %q and never the manifest's claim, got %q", tc.want, signer)
			}
			if problems := verifiedWith(t, export, tc.keyring).Problems; len(problems) != 0 {
				t.Fatalf("the same keyring should verify it: %v", problems)
			}
			matchesTheSchema(t, export)
		})
	}
}

func TestAnExportSignedByAKeyInTheKeyringNamesTheKeyringsSigner(t *testing.T) {
	ring := t.TempDir()
	from := signedRun(t, ring, "acme")
	keyring := keyringAt(t, ring)
	export := exported(t, from, audit.Options{Keyring: keyring})
	if signer := at(summary(t, export), "evidence", "signer"); signer != "acme" {
		t.Fatalf("want acme from the keyring, got %#v", signer)
	}
	if problems := verifiedWith(t, export, keyring).Problems; len(problems) != 0 {
		t.Fatalf("want it to verify, got %v", problems)
	}
}

// A verifier whose keyring cannot vouch for the name the export gives is shown
// both answers, rather than the export's name passing as a fact.
func TestAVerifierWhoseKeyringDisagreesIsToldBothAnswers(t *testing.T) {
	ring := t.TempDir()
	export := exported(t, signedRun(t, ring, "acme"), audit.Options{Keyring: keyringAt(t, ring)})
	problems := verified(t, export).Problems
	if !mentions(problems, `evidence.signer says "acme", and this keyring says "`+evidence.NoKeyring+`"`) {
		t.Fatalf("want the signer disagreement named, got %v", problems)
	}
	if mentions(problems, "audit.json: evidence is not") {
		t.Fatalf("the signer is reported once, by name: %v", problems)
	}
}

// An export written at version 1 said the manifest's signer claim, and it is
// read as version 1 still: it verifies as what it said, and no more.
func TestAVersionOneExportStillVerifiesAsVersionOne(t *testing.T) {
	export := exported(t, ran(t), audit.Options{})
	file := filepath.Join(export.Path(), audit.SummaryFile)
	record := summary(t, export)
	record["version"] = 1
	record["evidence"].(map[string]any)["signer"] = "a-test"
	text, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write(t, file, string(text))
	if problems := verified(t, export).Problems; len(problems) != 0 {
		t.Fatalf("a version 1 export should still verify: %v", problems)
	}
	record["evidence"].(map[string]any)["signer"] = "somebody else"
	text, _ = json.MarshalIndent(record, "", "  ")
	write(t, file, string(text))
	if problems := verified(t, export).Problems; !mentions(problems, "evidence.signer") {
		t.Fatalf("a version 1 signer that is not the manifest's claim should fail: %v", problems)
	}
}
