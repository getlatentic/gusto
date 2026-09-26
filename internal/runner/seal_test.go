package runner_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/runner"
	"github.com/getlatentic/haas/internal/signing"
	"github.com/getlatentic/haas/schema"
)

// keyed names a fresh key as the runner's and returns the keyring holding its
// public half.
func keyed(t *testing.T, name string) signing.Keyring {
	t.Helper()
	ring := t.TempDir()
	if _, err := signing.Generate(ring, name); err != nil {
		t.Fatal(err)
	}
	t.Setenv(signing.KeyEnv, filepath.Join(ring, name+".key"))
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	return keyring
}

func TestARunWithAKeySignsItsManifest(t *testing.T) {
	keyring := keyed(t, "acme")
	dir, _ := ran(t)
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
	t.Setenv(signing.KeyEnv, "")
	dir, _ := ran(t)
	if _, err := os.Stat(dir.OutFile(evidence.SignatureFile)); !os.IsNotExist(err) {
		t.Fatalf("no key, no signature: %v", err)
	}
	attested, err := evidence.Attest(dir, signing.Keyring{})
	if err != nil || attested.Signed {
		t.Fatalf("want unsigned, got %+v: %v", attested, err)
	}
}

// A runner with a key and no HAAS_SIGNER names the key in the manifest, so a
// signed record never calls itself unsigned.
func TestASignedManifestWithNoNameGivesTheKeysID(t *testing.T) {
	keyring := keyed(t, "acme")
	dir := unnamedRun(t)
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

func TestANamedKeyThatCannotBeReadStopsTheRunBeforeItStarts(t *testing.T) {
	t.Setenv(signing.KeyEnv, filepath.Join(t.TempDir(), "absent.key"))
	dir := unnamedJob(t)
	if _, err := runner.Inside(dir); err == nil || !strings.Contains(err.Error(), "absent.key") {
		t.Fatalf("want the key named, got %v", err)
	}
	if _, err := os.Stat(dir.OutFile("transcript.log")); !os.IsNotExist(err) {
		t.Fatalf("the harness should not have run: %v", err)
	}
}

func unnamedRun(t *testing.T) job.Dir {
	t.Helper()
	dir := unnamedJob(t)
	if _, err := runner.Inside(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// unnamedJob is the fixture job with HAAS_SIGNER unset.
func unnamedJob(t *testing.T) job.Dir {
	t.Helper()
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), adapter)
	t.Setenv("HAAS_HARNESSES", harnesses)
	t.Setenv("HAAS_SIGNER", "")
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    repo(t),
		Gate:    []string{"sh", "-c", `test "$(cat answer.txt)" = yes`},
		Harness: "fixture",
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}
