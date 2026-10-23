package evidence_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
)

// sealedUnder makes a job called id under root, signed by signer.
func sealedUnder(t *testing.T, root, id string, signer signing.Signer) job.Dir {
	t.Helper()
	dir := recordedAt(t, job.Dir(filepath.Join(root, id)), "answer of "+id)
	if err := evidence.Seal(dir, signer); err != nil {
		t.Fatal(err)
	}
	return dir
}

func signersOf(t *testing.T, root, ring string) evidence.Signers {
	t.Helper()
	found, err := evidence.SignersUnder(root, keyringOf(t, ring))
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// A key is replaced by making another and keeping the old public file. What the
// old key signed still verifies and still names it, and what the new one signs
// names the new one.
func TestARotatedKeyLeavesEveryEarlierRecordVerifiableAndNamed(t *testing.T) {
	ring, root := t.TempDir(), t.TempDir()
	older := keyIn(t, ring, "acme-2026-09")
	sealedUnder(t, root, "a", older)
	sealedUnder(t, root, "b", older)

	newer := keyIn(t, ring, "acme-2026-10")
	sealedUnder(t, root, "c", newer)

	found := signersOf(t, root, ring)
	if !found.Sound() || found.Signed["acme-2026-09"] != 2 || found.Signed["acme-2026-10"] != 1 {
		t.Fatalf("after a rotation: %+v", found)
	}
	attested, err := evidence.Attest(job.Dir(filepath.Join(root, "a")), keyringOf(t, ring))
	if err != nil || attested.Signer != "acme-2026-09" || attested.Key != older.ID() {
		t.Fatalf("an earlier record names %+v: %v", attested, err)
	}
}

// Leaving a retired key's public file out of the keyring does not break a
// record, it makes it unattributable, and the check says so by key id until the
// file is put back.
func TestARecordWhoseKeyLeftTheKeyringIsListedAndComesBackWithIt(t *testing.T) {
	ring, root := t.TempDir(), t.TempDir()
	older := keyIn(t, ring, "acme-2026-09")
	dir := sealedUnder(t, root, "a", older)
	sealedUnder(t, root, "b", keyIn(t, ring, "acme-2026-10"))

	public := filepath.Join(ring, "acme-2026-09.pub")
	held, err := os.ReadFile(public)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(public); err != nil {
		t.Fatal(err)
	}

	found := signersOf(t, root, ring)
	if found.Sound() || found.Unknown[older.ID()] != 1 || len(found.Broken) != 0 {
		t.Fatalf("with the key left out: %+v", found)
	}
	if wrong := complaints(t, dir); len(wrong) != 0 {
		t.Errorf("the record's own evidence broke: %v", wrong)
	}

	if err := os.WriteFile(public, held, 0o644); err != nil {
		t.Fatal(err)
	}
	if found := signersOf(t, root, ring); !found.Sound() || found.Signed["acme-2026-09"] != 1 {
		t.Fatalf("with the key put back: %+v", found)
	}
}

func TestATamperedManifestIsBrokenAndNotMerelyUnknown(t *testing.T) {
	ring, root := t.TempDir(), t.TempDir()
	dir := sealedUnder(t, root, "a", keyIn(t, ring, "acme"))
	sealedUnder(t, root, "b", keyIn(t, ring, "other"))
	manifest := dir.OutFile("manifest.json")
	text, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, append(text, ' '), 0o644); err != nil {
		t.Fatal(err)
	}

	found := signersOf(t, root, ring)
	if found.Sound() || len(found.Broken) != 1 || found.Broken[0] != "a" || len(found.Unknown) != 0 {
		t.Fatalf("with one record altered: %+v", found)
	}
	if found.Signed["other"] != 1 {
		t.Errorf("the untouched record was not counted: %+v", found)
	}
}

func TestAnUnsignedRecordIsCountedAndDoesNotFailTheCheck(t *testing.T) {
	ring, root := t.TempDir(), t.TempDir()
	keyIn(t, ring, "acme")
	recordedAt(t, job.Dir(filepath.Join(root, "a")), "unsigned")
	if found := signersOf(t, root, ring); !found.Sound() || found.Unsigned != 1 {
		t.Fatalf("with an unsigned record: %+v", found)
	}
	if found, err := evidence.SignersUnder(filepath.Join(root, "nothing-here"), signing.Keyring{}); err != nil || !found.Sound() {
		t.Fatalf("a directory that is not there: %+v %v", found, err)
	}
}

// The keys directory is lost with the machine. A copy of it on a clean one
// brings back the same key: what it signed before verifies, and what it signs
// next names the same signer with the same id.
func TestAKeyRestoredFromABackupOnACleanMachineSignsAsTheSameKey(t *testing.T) {
	keys, root, backup := t.TempDir(), t.TempDir(), t.TempDir()
	original := keyIn(t, keys, "acme")
	sealedUnder(t, root, "before", original)
	copyFiles(t, keys, backup)

	if err := os.RemoveAll(keys); err != nil {
		t.Fatal(err)
	}
	clean := t.TempDir()
	copyFiles(t, backup, clean)

	restored, err := signing.ReadKey(filepath.Join(clean, "acme.key"))
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID() != original.ID() {
		t.Fatalf("the restored key is %s, and the original was %s", restored.ID(), original.ID())
	}
	info, err := os.Stat(filepath.Join(clean, "acme.key"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the restored private key is %v: %v", info, err)
	}
	sealedUnder(t, root, "after", restored)

	if found := signersOf(t, root, clean); !found.Sound() || found.Signed["acme"] != 2 {
		t.Fatalf("after the restore: %+v", found)
	}
}

// copyFiles copies every file in from to a directory that exists, keeping each
// file's permissions, as a backup and a restore do.
func copyFiles(t *testing.T, from, to string) {
	t.Helper()
	listing, err := os.ReadDir(from)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listing {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		text, err := os.ReadFile(filepath.Join(from, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(to, entry.Name()), text, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
}
