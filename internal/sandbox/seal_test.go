package sandbox

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/signing"
)

// hostKey makes a key on the host and names it as the one to sign with.
func hostKey(t *testing.T) (string, signing.Keyring) {
	t.Helper()
	ring := t.TempDir()
	key, err := signing.Generate(ring, "host")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ring, "host.key")
	t.Setenv(signing.KeyEnv, path)
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := keyring[key.ID()]; !found {
		t.Fatal("the keyring should hold the key just made")
	}
	return path, keyring
}

// The key signs outside the box, so nothing on the container's command line
// may lead to it: not its path, not the variable that names it.
func TestTheSigningKeyNeverEntersTheContainer(t *testing.T) {
	path, keyring := hostKey(t)
	t.Setenv("HAAS_SIGNER", "")
	args, err := argv("/abs/job", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(args, " ")
	if strings.Contains(line, path) || strings.Contains(line, signing.KeyEnv) {
		t.Fatalf("the key leaks into the container: %s", line)
	}
	for id := range keyring {
		if !strings.Contains(line, "HAAS_SIGNER="+id) {
			t.Fatalf("with no name, the box should name the host's key %s: %s", id, line)
		}
	}
}

func TestAHostKeyThatCannotBeReadStopsTheRunBeforeTheBox(t *testing.T) {
	t.Setenv(signing.KeyEnv, filepath.Join(t.TempDir(), "absent.key"))
	if _, err := argv("/abs/job", nil, nil); err == nil || !strings.Contains(err.Error(), "absent.key") {
		t.Fatalf("want the key named, got %v", err)
	}
}

// The box writes the manifest and the host signs it once the box is gone. A
// runtime that does nothing stands in for the box that already wrote one.
func TestTheHostSignsTheManifestTheBoxLeft(t *testing.T) {
	_, keyring := hostKey(t)
	t.Setenv("HAAS_CONTAINER", "true")
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{Signer: "the box"}); err != nil {
		t.Fatal(err)
	}
	if _, err := start(dir, nil); err != nil {
		t.Fatal(err)
	}
	attested, err := evidence.Attest(dir, keyring)
	if err != nil || attested.Signer != "host" {
		t.Fatalf("want the host's key, got %+v: %v", attested, err)
	}
}
