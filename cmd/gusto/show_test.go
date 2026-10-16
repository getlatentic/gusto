package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/signing"
)

// claimed is a finished job whose manifest says "alice" signed it, a claim
// nothing checks.
func claimed(t *testing.T) job.Dir {
	t.Helper()
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "b", Repo: "r", Gate: []string{"true"}, Harness: "h"}
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

func sealedBy(t *testing.T, dir job.Dir, ring, name string) {
	t.Helper()
	key, err := signing.Generate(ring, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Seal(dir, key); err != nil {
		t.Fatal(err)
	}
}

// shown is the signer line gusto show prints under the keyring at ring, which
// is empty for none.
func shown(t *testing.T, dir job.Dir, ring string) string {
	t.Helper()
	t.Setenv(signing.KeyringEnv, ring)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = writer
	code := run([]string{"show", dir.Path()})
	os.Stdout = stdout
	writer.Close()
	text, err := io.ReadAll(reader)
	if err != nil || code != 0 {
		t.Fatalf("show exited %d: %v", code, err)
	}
	for _, line := range strings.Split(string(text), "\n") {
		if signer, found := strings.CutPrefix(line, "signer   "); found {
			return signer
		}
	}
	t.Fatalf("show printed no signer:\n%s", text)
	return ""
}

func TestShowNamesOnlyTheSignerTheKeyringChecked(t *testing.T) {
	trusted := t.TempDir()
	if _, err := signing.Generate(trusted, "acme"); err != nil {
		t.Fatal(err)
	}
	unsigned := claimed(t)
	stranger := claimed(t)
	sealedBy(t, stranger, t.TempDir(), "stranger")
	acme := claimed(t)
	sealedBy(t, acme, trusted, "acme-2026")
	for name, tc := range map[string]struct {
		dir  job.Dir
		ring string
		want string
	}{
		"unsigned":                           {unsigned, trusted, evidence.Unsigned},
		"signed, no keyring":                 {stranger, "", evidence.NoKeyring},
		"signed by a key not in the keyring": {stranger, trusted, "is not in the keyring"},
		"signed by a key in the keyring":     {acme, trusted, "acme-2026"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := shown(t, tc.dir, tc.ring); !strings.Contains(got, tc.want) || strings.Contains(got, "alice") {
				t.Fatalf("want %q and never the manifest's claim, got %q", tc.want, got)
			}
		})
	}
}

// A turn in a conversation has no gate, and gusto show says so instead of failing on the manifest.
func TestShowAJobWithNoGate(t *testing.T) {
	dir := claimed(t)
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Gate = nil
	if err := evidence.Write(dir, manifest); err != nil {
		t.Fatal(err)
	}
	if got := shown(t, dir, ""); got == "" {
		t.Fatal("gusto show printed no signer line for a job with no gate")
	}
}
