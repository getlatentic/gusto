package signing_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/signing"
)

// generated makes a key pair in a keyring directory and returns the key.
func generated(t *testing.T, ring, name string) signing.Key {
	t.Helper()
	key, err := signing.Generate(ring, name)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestAGeneratedPrivateKeyIsReadableByItsOwnerAlone(t *testing.T) {
	ring := t.TempDir()
	key := generated(t, ring, "acme")
	info, err := os.Stat(filepath.Join(ring, "acme.key"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("the private key is %o, want 600", mode)
	}
	read, err := signing.ReadKey(filepath.Join(ring, "acme.key"))
	if err != nil {
		t.Fatal(err)
	}
	if read.ID() != key.ID() || !strings.HasPrefix(key.ID(), "sha256:") {
		t.Fatalf("read back %s, generated %s", read.ID(), key.ID())
	}
}

func TestGenerateRefusesToReplaceAKey(t *testing.T) {
	ring := t.TempDir()
	generated(t, ring, "acme")
	before, err := os.ReadFile(filepath.Join(ring, "acme.key"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := signing.Generate(ring, "acme"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("want the existing key refused, got %v", err)
	}
	after, err := os.ReadFile(filepath.Join(ring, "acme.key"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("the key was touched: %v", err)
	}
}

func TestASignersNameIsAPlainFileName(t *testing.T) {
	for _, name := range []string{"", ".hidden", "../elsewhere", `a\b`} {
		if _, err := signing.Generate(t.TempDir(), name); err == nil {
			t.Errorf("want %q refused", name)
		}
	}
}

func TestASignatureHoldsForTheBytesItSignedAndNoOthers(t *testing.T) {
	key := generated(t, t.TempDir(), "acme")
	signature := signing.Sign(key, []byte("the record"))
	if err := signature.Intact([]byte("the record")); err != nil {
		t.Fatal(err)
	}
	if err := signature.Intact([]byte("the recorD")); !errors.Is(err, signing.ErrMismatch) {
		t.Fatalf("want a changed byte refused, got %v", err)
	}
}

func TestASignatureNamingAKeyOtherThanTheOneItCarriesIsRefused(t *testing.T) {
	ring := t.TempDir()
	carried := generated(t, ring, "acme")
	named := generated(t, ring, "other")
	signature := signing.Sign(carried, []byte("the record"))
	signature.Key = named.ID()
	if err := signature.Intact([]byte("the record")); err == nil || !strings.Contains(err.Error(), named.ID()) {
		t.Fatalf("want the mismatched id named, got %v", err)
	}
}

func TestTheKeyringNamesTheSignerFromItsOwnFile(t *testing.T) {
	ring := t.TempDir()
	key := generated(t, ring, "acme-2026")
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := keyring.Signer(signing.Sign(key, []byte("the record")), []byte("the record"))
	if err != nil || signer.Name != "acme-2026" {
		t.Fatalf("signer %q: %v", signer.Name, err)
	}
}

func TestAKeyNotInTheKeyringIsRefusedByItsID(t *testing.T) {
	stranger := generated(t, t.TempDir(), "stranger")
	ring := t.TempDir()
	generated(t, ring, "acme")
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	_, err = keyring.Signer(signing.Sign(stranger, []byte("r")), []byte("r"))
	if err == nil || !strings.Contains(err.Error(), stranger.ID()) || !strings.Contains(err.Error(), "not in the keyring") {
		t.Fatalf("want the stranger's key named, got %v", err)
	}
}

// A signature can claim a trusted key's id while its value came from another
// key; the keyring checks against its own copy, so the claim buys nothing.
func TestTheKeyringChecksItsOwnCopyOfTheKey(t *testing.T) {
	ring := t.TempDir()
	trusted := generated(t, ring, "acme")
	stranger := generated(t, t.TempDir(), "stranger")
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	forged := signing.Sign(stranger, []byte("r"))
	forged.Key = trusted.ID()
	if _, err := keyring.Signer(forged, []byte("r")); !errors.Is(err, signing.ErrMismatch) {
		t.Fatalf("want the forged signature refused, got %v", err)
	}
}

func TestAKeyringHoldingOneKeyTwiceIsRefused(t *testing.T) {
	ring := t.TempDir()
	generated(t, ring, "acme")
	public, err := os.ReadFile(filepath.Join(ring, "acme.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ring, "impostor.pub"), public, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := signing.OpenKeyring(ring); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("want the duplicate refused, got %v", err)
	}
}

func TestASignatureFileRoundTripsAndANewerVersionIsRefused(t *testing.T) {
	key := generated(t, t.TempDir(), "acme")
	text, err := signing.Sign(key, []byte("r")).Encode()
	if err != nil {
		t.Fatal(err)
	}
	read, err := signing.Decode("manifest.sig", text)
	if err != nil || read.Intact([]byte("r")) != nil {
		t.Fatalf("round trip: %v", err)
	}
	newer := strings.Replace(string(text), `"version": 1`, `"version": 2`, 1)
	if _, err := signing.Decode("manifest.sig", []byte(newer)); err == nil || !strings.Contains(err.Error(), "manifest.sig is version 2") {
		t.Fatalf("want the version named, got %v", err)
	}
}

func TestNoKeyConfiguredLeavesARunUnsignedAndABadOneFails(t *testing.T) {
	t.Setenv(signing.KeyEnv, "")
	if _, found, err := signing.Configured(); found || err != nil {
		t.Fatalf("found %t: %v", found, err)
	}
	t.Setenv(signing.KeyEnv, filepath.Join(t.TempDir(), "absent.key"))
	if _, found, err := signing.Configured(); found || err == nil {
		t.Fatalf("want a named but missing key refused, found %t", found)
	}
}

// A key handed out by name is the same key a keyring reads back from the file.
func TestAKeyHandedOutByNameReadsBackAsTheSameKey(t *testing.T) {
	dir := t.TempDir()
	if _, err := signing.Generate(dir, "greencloud"); err != nil {
		t.Fatal(err)
	}
	ring, err := signing.OpenKeyring(dir)
	if err != nil {
		t.Fatal(err)
	}
	held, found := ring.Named("greencloud")
	if !found {
		t.Fatal("the keyring should find the key by its name")
	}
	text, err := held.PEM()
	if err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, "greencloud.pub"), text, 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := signing.OpenKeyring(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := again[signing.IDOf(held.Public)]; !found {
		t.Error("the handed-out key should be the one the keyring holds")
	}
	if _, found := ring.Named("someone-else"); found {
		t.Error("a name the keyring does not hold should not be found")
	}
}
