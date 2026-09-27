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
	})
	dir := job.Dir(filepath.Join(root, id))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "b", Repo: "/repo", Gate: []string{"true"}, Harness: "fixture"}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	hash, files, err := evidence.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest := evidence.Manifest{Job: hash, Files: files, Gate: &evidence.Verdict{Command: spec.Gate, Passed: true}, Signer: "alice"}
	if err := evidence.Write(dir, manifest); err != nil {
		t.Fatal(err)
	}
	return dir
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
		"unsigned":                           {"unsigned-20260927-0100", keyring, "Not signed"},
		"signed, no keyring":                 {"acme-20260927-0100", signing.Keyring{}, evidence.NoKeyring},
		"signed by a key not in the keyring": {"stranger-20260927-0100", keyring, "is not in the keyring"},
		"signed by a key in the keyring":     {"acme-20260927-0100", keyring, "Signed by acme."},
	} {
		t.Run(name, func(t *testing.T) {
			site := openKeyed(t, root, tc.keyring)
			site.signIn(t)
			body := site.ask(t, "GET", "/jobs/"+tc.id, nil).Body.String()
			if !strings.Contains(body, tc.want) || strings.Contains(body, "alice") {
				t.Fatalf("want %q in the proof and never the manifest's claim:\n%s", tc.want, body)
			}
		})
	}
}

// A verifier fetches the signer's public key before trusting anything else
// the server says, so the key is served without signing in.
func TestTheSignersKeyIsServedToAnyone(t *testing.T) {
	root, trusted := t.TempDir(), t.TempDir()
	sealWith(t, claiming(t, root, "acme-20260927-0100"), trusted, "acme")
	site := openKeyed(t, root, keyringAt(t, trusted))
	got := site.ask(t, "GET", "/keys/acme.pub", nil)
	if got.Code != 200 || !strings.Contains(got.Body.String(), "BEGIN PUBLIC KEY") {
		t.Fatalf("the key should be served, got %d:\n%s", got.Code, got.Body.String())
	}
	if missing := site.ask(t, "GET", "/keys/stranger.pub", nil); missing.Code != 404 {
		t.Errorf("a key the keyring does not hold gave %d, want 404", missing.Code)
	}
}
