package ship_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/ship"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
)

// signedBy finishes the job signed by a key kept in ring as name. The manifest
// still claims "a-test", which no keyring vouches for.
func signedBy(t *testing.T, ring, name string) job.Dir {
	t.Helper()
	if _, err := signing.Generate(ring, name); err != nil {
		t.Fatal(err)
	}
	t.Setenv(signing.KeyEnv, filepath.Join(ring, name+".key"))
	dir, _ := finished(t, "yes")
	return dir
}

func keyringAt(t *testing.T, ring string) signing.Keyring {
	t.Helper()
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	return keyring
}

func TestThePullRequestNamesOnlyTheSignerTheKeyringChecked(t *testing.T) {
	trusted := t.TempDir()
	if _, err := signing.Generate(trusted, "acme"); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		from    func(t *testing.T) job.Dir
		keyring signing.Keyring
		want    string
	}{
		"unsigned": {
			from:    func(t *testing.T) job.Dir { dir, _ := finished(t, "yes"); return dir },
			keyring: keyringAt(t, trusted), want: "| signer | " + evidence.Unsigned + " |",
		},
		"signed, no keyring": {
			from:    func(t *testing.T) job.Dir { return signedBy(t, t.TempDir(), "stranger") },
			keyring: signing.Keyring{}, want: "| signer | " + evidence.NoKeyring + " |",
		},
		"signed by a key not in the keyring": {
			from:    func(t *testing.T) job.Dir { return signedBy(t, t.TempDir(), "stranger") },
			keyring: keyringAt(t, trusted), want: "is not in the keyring |",
		},
	} {
		t.Run(name, func(t *testing.T) {
			body := bodyFor(t, tc.from(t), tc.keyring)
			if !strings.Contains(body, tc.want) || strings.Contains(body, "a-test") {
				t.Fatalf("want %q and never the manifest's claim:\n%s", tc.want, body)
			}
		})
	}
}

func TestThePullRequestNamesTheKeyringsSigner(t *testing.T) {
	ring := t.TempDir()
	dir := signedBy(t, ring, "acme")
	if body := bodyFor(t, dir, keyringAt(t, ring)); !strings.Contains(body, "| signer | acme |") {
		t.Fatalf("want acme from the keyring:\n%s", body)
	}
}

func bodyFor(t *testing.T, dir job.Dir, keyring signing.Keyring) string {
	t.Helper()
	options := here
	options.Keyring = keyring
	plan, err := ship.Prepare(dir, options)
	if err != nil {
		t.Fatal(err)
	}
	return plan.Body
}
