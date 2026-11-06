package sandbox

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
)

// hostKey makes a key on the host and opens it as the one to sign with.
func hostKey(t *testing.T) (string, signing.Keyring, signing.Identity) {
	t.Helper()
	ring := t.TempDir()
	key, err := signing.Generate(ring, "host")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ring, "host.key")
	identity, err := signing.Open(context.Background(), path, "")
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := keyring[key.ID()]; !found {
		t.Fatal("the keyring should hold the key just made")
	}
	return path, keyring, identity
}

// The key signs outside the box, so nothing on the container's command line
// may lead to it: not its path, not the variable that names it.
func TestTheSigningKeyNeverEntersTheContainer(t *testing.T) {
	t.Parallel()
	path, keyring, identity := hostKey(t)
	c := plain()
	c.Signing = identity
	args, err := c.argv("/abs/job", nil, Owner{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Join(args, " ")
	if strings.Contains(line, path) || strings.Contains(line, signing.KeyEnv) {
		t.Fatalf("the key leaks into the container: %s", line)
	}
	for id := range keyring {
		if !strings.Contains(line, "GUSTO_SIGNER="+id) {
			t.Fatalf("with no name, the box should name the host's key %s: %s", id, line)
		}
	}
}

// The box writes the manifest and the host signs it once the box is gone. A
// runtime that does nothing stands in for the box that already wrote one.
func TestTheHostSignsTheManifestTheBoxLeft(t *testing.T) {
	t.Parallel()
	_, keyring, identity := hostKey(t)
	c := with("true", "gusto:dev")
	c.Signing = identity
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{Signer: "the box"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.start(context.Background(), dir, nil); err != nil {
		t.Fatal(err)
	}
	attested, err := evidence.Attest(dir, keyring)
	if err != nil || attested.Signer != "host" {
		t.Fatalf("want the host's key, got %+v: %v", attested, err)
	}
}
