package record_test

import (
	"slices"
	"testing"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/pkg/task"
)

// A room's turn is decided after its record was signed: the decision goes under the signature, which still holds.
func TestADecisionAddedAfterARunIsHashedAndSignedWithTheRun(t *testing.T) {
	ring := t.TempDir()
	key := keyIn(t, ring, "acme")
	dir := aRoomTurn(t)
	allowance := 5.0
	if err := record.WriteRun(dir, record.Run{Asked: "bob@acme.com", PaidBy: "ada@acme.com", Agent: "a1", AgentName: "Scout",
		Room: "room-launch", Sandbox: "c-ada", AllowanceUSD: &allowance}); err != nil {
		t.Fatal(err)
	}
	spec, files, err := record.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Write(dir, record.Manifest{Task: spec, Files: files}); err != nil {
		t.Fatal(err)
	}
	if err := record.SealIfKeyed(dir, key); err != nil {
		t.Fatal(err)
	}

	if err := record.Decide(dir, record.Decision{Kept: true, Changed: 2, Joined: []string{"plan.md"}}, key); err != nil {
		t.Fatal(err)
	}

	manifest, err := record.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	var named []string
	for _, entry := range manifest.Files {
		named = append(named, entry.Name)
	}
	if !slices.Contains(named, record.RunFile) || !slices.Contains(named, record.DecisionFile) {
		t.Errorf("the manifest names %v", named)
	}
	if wrong := complaints(t, dir); len(wrong) != 0 {
		t.Errorf("the record does not check: %v", wrong)
	}
	if attested, err := record.Attest(dir, keyringOf(t, ring)); err != nil || !attested.Signed || attested.Signer != "acme" {
		t.Errorf("the record is %+v, %v", attested, err)
	}
}

func aRoomTurn(t *testing.T) task.Dir {
	t.Helper()
	dir := task.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir.Spec(), `{"brief": "b", "harness": "h", "session": "room-launch", "agent": "a1"}`)
	return dir
}
