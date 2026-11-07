package evidence_test

import (
	"errors"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/task"
)

// aSignedTurn is a room's turn whose run was hashed and signed with the acme key, as a run ends.
func aSignedTurn(t *testing.T) (dir task.Dir, ring string, key signing.Key) {
	t.Helper()
	ring = t.TempDir()
	key = keyIn(t, ring, "acme")
	dir = aRoomTurn(t)
	if err := evidence.WriteRun(dir, evidence.Run{Asked: "bob@acme.com", Agent: "a1", Room: "room-launch"}); err != nil {
		t.Fatal(err)
	}
	spec, files, err := evidence.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{Task: spec, Files: files}); err != nil {
		t.Fatal(err)
	}
	if err := evidence.SealIfKeyed(dir, key); err != nil {
		t.Fatal(err)
	}
	return dir, ring, key
}

// brokenKey is a key that cannot sign, as a KMS key does when it is out of reach.
type brokenKey struct{ signing.Key }

func (brokenKey) Sign([]byte) ([]byte, error) { return nil, errors.New("the key is out of reach") }

func signedBy(t *testing.T, dir task.Dir, ring string) string {
	t.Helper()
	attested, err := evidence.Attest(dir, keyringOf(t, ring))
	if err != nil {
		t.Fatal(err)
	}
	if !attested.Signed {
		return ""
	}
	return attested.Signer
}

// A key that cannot sign the decision leaves the record signed as the run left it. spec/Evidence.before-pending.cfg is the
// trace of the crash at the same step.
func TestADecisionTheKeyCannotSignLeavesTheRecordSignedAsItWas(t *testing.T) {
	t.Parallel()
	dir, ring, key := aSignedTurn(t)
	if err := evidence.Decide(dir, evidence.Decision{Kept: true, Changed: 1}, brokenKey{key}); err == nil {
		t.Fatal("a decision no key could sign was taken")
	}
	if signer := signedBy(t, dir, ring); signer != "acme" {
		t.Errorf("the record is signed by %q", signer)
	}
	if wrong := complaints(t, dir); len(wrong) != 0 {
		t.Errorf("the record does not check: %v", wrong)
	}
}

// A file changed after the run was signed is not signed again by the decision. spec/Evidence.before-intact.cfg is the trace.
func TestARecordChangedSinceItWasSignedIsNotSignedAgain(t *testing.T) {
	dir, ring, key := aSignedTurn(t)
	writeFile(t, dir.OutFile(evidence.RunFile), `{"asked": "mallory@acme.com"}`)
	if err := evidence.Decide(dir, evidence.Decision{Kept: true}, key); err == nil {
		t.Fatal("a changed record was decided")
	}
	if wrong := complaints(t, dir); !slices.Contains(wrong, evidence.RunFile+" changed") {
		t.Errorf("the change is not said: %v", wrong)
	}
	if signer := signedBy(t, dir, ring); signer != "acme" {
		t.Errorf("the record is signed by %q", signer)
	}
}

// A decision cut short after it was made is said by the check, and a start puts it in place under the signature.
func TestADecisionCutShortIsSaidAndFinished(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a folder's mode stops writes only for a person who is not root, on Unix")
	}
	dir, ring, key := aSignedTurn(t)
	if err := os.Chmod(dir.Out(), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir.Out(), 0o755) })
	if err := evidence.Decide(dir, evidence.Decision{Kept: true, Changed: 3}, key); err == nil {
		t.Fatal("a decision out/ could not take was put in place")
	}
	os.Chmod(dir.Out(), 0o755)
	wrong := complaints(t, dir)
	if !slices.ContainsFunc(wrong, func(w string) bool { return strings.Contains(w, evidence.PendingFile) }) {
		t.Fatalf("the check does not say a decision is waiting: %v", wrong)
	}
	if err := evidence.Finish(dir); err != nil {
		t.Fatal(err)
	}
	if wrong := complaints(t, dir); len(wrong) != 0 {
		t.Errorf("the finished record does not check: %v", wrong)
	}
	if signer := signedBy(t, dir, ring); signer != "acme" {
		t.Errorf("the finished record is signed by %q", signer)
	}
	if text, err := os.ReadFile(dir.OutFile(evidence.DecisionFile)); err != nil || !strings.Contains(string(text), `"changed": 3`) {
		t.Errorf("the decision is %q, %v", text, err)
	}
}
