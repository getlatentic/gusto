package evidence_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
)

// A room's turn is decided after its record was signed: the decision goes under the signature, which still holds.
func TestADecisionAddedAfterARunIsHashedAndSignedWithTheRun(t *testing.T) {
	ring := t.TempDir()
	keyIn(t, ring, "acme")
	t.Setenv(signing.KeyEnv, filepath.Join(ring, "acme.key"))
	dir := aRoomTurn(t)
	allowance := 5.0
	if err := evidence.WriteRun(dir, evidence.Run{Asked: "bob@acme.com", PaidBy: "ada@acme.com", Agent: "a1", AgentName: "Scout",
		Room: "room-launch", Sandbox: "c-ada", AllowanceUSD: &allowance}); err != nil {
		t.Fatal(err)
	}
	spec, files, err := evidence.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{Job: spec, Files: files}); err != nil {
		t.Fatal(err)
	}
	if err := evidence.SealAsConfigured(dir); err != nil {
		t.Fatal(err)
	}

	if err := evidence.Decide(dir, evidence.Decision{Kept: true, Changed: 2, Joined: []string{"plan.md"}}); err != nil {
		t.Fatal(err)
	}

	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	var named []string
	for _, entry := range manifest.Files {
		named = append(named, entry.Name)
	}
	if !slices.Contains(named, evidence.RunFile) || !slices.Contains(named, evidence.DecisionFile) {
		t.Errorf("the manifest names %v", named)
	}
	if wrong := complaints(t, dir); len(wrong) != 0 {
		t.Errorf("the record does not check: %v", wrong)
	}
	if attested, err := evidence.Attest(dir, keyringOf(t, ring)); err != nil || !attested.Signed || attested.Signer != "acme" {
		t.Errorf("the record is %+v, %v", attested, err)
	}
}

func aRoomTurn(t *testing.T) job.Dir {
	t.Helper()
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir.Spec(), `{"brief": "b", "harness": "h", "session": "room-launch", "agent": "a1"}`)
	return dir
}
