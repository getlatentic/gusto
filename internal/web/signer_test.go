package web

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/signing"
)

// claiming writes a finished job whose manifest says "alice" signed it, a
// claim nothing checks, and returns its directory.
func claiming(t *testing.T, root, id string) job.Dir {
	t.Helper()
	leaveOld(t, filepath.Join(root, id), map[string]string{
		"state.json": `{"id":"` + id + `","state":"passed","brief":"b","repo":"/repo","harness":"fixture",` +
			`"asked":"someone","exit":0,"queuedAt":1788264000,"startedAt":1788264001,"endedAt":1788264060}`,
		"out/manifest.json": `{"version":1,"job":"","files":[],"gate":{"command":["true"],"code":0,"passed":true},` +
			`"harness":"fixture","harnessExit":0,"stoppedAtBudget":false,"modelEndpoint":"local",` +
			`"tokens":null,"costUsd":null,"signer":"alice","atUnix":1788264060}`,
	})
	return job.Dir(filepath.Join(root, id))
}

func sealWith(t *testing.T, dir job.Dir, ring, name string) {
	t.Helper()
	key, err := signing.Generate(ring, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Seal(dir, key); err != nil {
		t.Fatal(err)
	}
}

func keyringAt(t *testing.T, ring string) signing.Keyring {
	t.Helper()
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	return keyring
}

func TestTheJobPageNamesOnlyTheSignerTheKeyringChecked(t *testing.T) {
	root, trusted := t.TempDir(), t.TempDir()
	claiming(t, root, "unsigned-20260927-0100")
	sealWith(t, claiming(t, root, "stranger-20260927-0100"), t.TempDir(), "stranger")
	sealWith(t, claiming(t, root, "acme-20260927-0100"), trusted, "acme")
	keyring := keyringAt(t, trusted)
	for name, tc := range map[string]struct {
		id      string
		keyring signing.Keyring
		want    string
	}{
		"unsigned":                           {"unsigned-20260927-0100", keyring, evidence.Unsigned},
		"signed, no keyring":                 {"acme-20260927-0100", signing.Keyring{}, evidence.NoKeyring},
		"signed by a key not in the keyring": {"stranger-20260927-0100", keyring, "is not in the keyring"},
		"signed by a key in the keyring":     {"acme-20260927-0100", keyring, "<td>acme</td>"},
	} {
		t.Run(name, func(t *testing.T) {
			site := openKeyed(t, root, tc.keyring)
			site.signIn(t)
			body := site.ask(t, "GET", "/jobs/"+tc.id, nil).Body.String()
			if !strings.Contains(body, "Signed by") || !strings.Contains(body, tc.want) || strings.Contains(body, "alice") {
				t.Fatalf("want %q beside Signed by and never the manifest's claim:\n%s", tc.want, body)
			}
		})
	}
}
