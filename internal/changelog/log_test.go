package changelog_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/signing"
)

// opened is a log signed by a key whose public half is in the keyring.
func opened(t *testing.T) (*changelog.Log, string, signing.Keyring) {
	t.Helper()
	ring := t.TempDir()
	key, err := signing.Generate(ring, "acme")
	if err != nil {
		t.Fatal(err)
	}
	keyring, err := signing.OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "changes", "changes.jsonl")
	log, err := changelog.Open(path, key, keyring)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	return log, path, keyring
}

func wrote(t *testing.T, log *changelog.Log, kinds ...string) {
	t.Helper()
	for _, kind := range kinds {
		if _, err := log.Append(changelog.Event{Who: "alice@example.com", Role: "admin", Kind: kind, What: map[string]string{"setup": "reviewer"}}); err != nil {
			t.Fatal(err)
		}
	}
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(text), "\n"), "\n")
}

func put(t *testing.T, path string, all []string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(all, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func verify(t *testing.T, path string, keyring signing.Keyring) (changelog.Report, error) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	return changelog.Verify(file, keyring)
}

func brokenAt(t *testing.T, err error, line int) {
	t.Helper()
	var broken *changelog.BrokenError
	if !errors.As(err, &broken) || broken.Line != line {
		t.Fatalf("want a break at line %d, got %v", line, err)
	}
}

func TestEventsAreNumberedChainedAndSigned(t *testing.T) {
	log, path, keyring := opened(t)
	wrote(t, log, changelog.SetupSaved, changelog.SkillWritten, changelog.SkillDropped)

	report, err := verify(t, path, keyring)
	if err != nil {
		t.Fatal(err)
	}
	if report.Events != 3 || report.Signed != 3 || report.Unsigned != 0 || report.Signers["acme"] != 3 {
		t.Fatalf("%+v", report)
	}
	all := lines(t, path)
	var last changelog.Event
	if err := json.Unmarshal([]byte(all[2]), &last); err != nil {
		t.Fatal(err)
	}
	if last.Seq != 3 || last.Hash != report.Head || last.At == "" || last.Prev == "" {
		t.Errorf("the last line: %+v", last)
	}
}

func TestALogOpenedAgainCarriesOnTheChain(t *testing.T) {
	log, path, keyring := opened(t)
	wrote(t, log, changelog.SetupSaved, changelog.SkillWritten)
	log.Close()

	ring := t.TempDir()
	key, _ := signing.Generate(ring, "acme")
	again, err := changelog.Open(path, key, keyring)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	wrote(t, again, changelog.SkillDropped)
	if report, err := verify(t, path, keyring); err != nil || report.Events != 3 {
		t.Fatalf("%+v %v", report, err)
	}
}

func TestAnEditedLineBreaksTheChainAtThatLine(t *testing.T) {
	log, path, keyring := opened(t)
	wrote(t, log, changelog.SetupSaved, changelog.SkillWritten, changelog.SkillDropped)
	all := lines(t, path)
	all[1] = strings.Replace(all[1], "alice@example.com", "mallory@example.com", 1)
	put(t, path, all)

	_, err := verify(t, path, keyring)
	brokenAt(t, err, 2)
}

func TestARemovedInsertedOrMovedLineBreaksTheChain(t *testing.T) {
	log, path, keyring := opened(t)
	wrote(t, log, changelog.SetupSaved, changelog.SkillWritten, changelog.SkillDropped, changelog.SetupSaved)
	original := lines(t, path)

	for name, change := range map[string]struct {
		lines []string
		at    int
	}{
		"removed":  {[]string{original[0], original[2], original[3]}, 2},
		"inserted": {[]string{original[0], original[1], original[1], original[2], original[3]}, 3},
		"moved":    {[]string{original[0], original[2], original[1], original[3]}, 2},
	} {
		put(t, path, change.lines)
		_, err := verify(t, path, keyring)
		if err == nil {
			t.Errorf("%s: the chain still held", name)
			continue
		}
		brokenAt(t, err, change.at)
	}
}

// Somebody who can write the file can edit a line and recompute its hash, so
// the hash alone would let the edit through. The signature is what they cannot
// make.
func TestALineRewrittenWithAFreshHashStillFailsItsSignature(t *testing.T) {
	log, path, keyring := opened(t)
	wrote(t, log, changelog.SetupSaved, changelog.SkillWritten)
	all := lines(t, path)
	var last changelog.Event
	if err := json.Unmarshal([]byte(all[1]), &last); err != nil {
		t.Fatal(err)
	}
	last.Who = "mallory@example.com"
	fresh, err := changelog.HashOf(last)
	if err != nil {
		t.Fatal(err)
	}
	last.Hash = fresh
	forged, _ := json.Marshal(last)
	all[1] = string(forged)
	put(t, path, all)

	report, err := verify(t, path, keyring)
	brokenAt(t, err, 2)
	if !strings.Contains(err.Error(), "signature") {
		t.Errorf("the break should be the signature: %v", err)
	}
	if report.Events != 1 {
		t.Errorf("the line before the forgery was not counted: %+v", report)
	}
}

// A chain cannot show a tail that was cut off. A copy of the head taken earlier
// can, which is what an export is for.
func TestATailCutOffLeavesAValidChainAndADifferentHead(t *testing.T) {
	log, path, keyring := opened(t)
	wrote(t, log, changelog.SetupSaved, changelog.SkillWritten, changelog.SkillDropped)
	before, err := verify(t, path, keyring)
	if err != nil {
		t.Fatal(err)
	}
	all := lines(t, path)
	put(t, path, all[:2])

	after, err := verify(t, path, keyring)
	if err != nil {
		t.Fatalf("a shorter log is still a chain: %v", err)
	}
	if after.Events != 2 || after.Head == before.Head {
		t.Errorf("before %+v, after %+v: the saved head should no longer match", before, after)
	}
}

func TestALogThatDoesNotHoldTogetherIsNotOpenedForAppending(t *testing.T) {
	log, path, keyring := opened(t)
	wrote(t, log, changelog.SetupSaved, changelog.SkillWritten)
	log.Close()
	all := lines(t, path)
	all[0] = strings.Replace(all[0], "reviewer", "elsewhere", 1)
	put(t, path, all)
	tampered, _ := os.ReadFile(path)

	_, err := changelog.Open(path, nil, keyring)
	brokenAt(t, err, 1)
	if now, _ := os.ReadFile(path); !bytes.Equal(now, tampered) {
		t.Error("opening a broken log changed it")
	}
}

func TestALogWithNoKeyIsChainedAndSaysItIsUnsigned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changes.jsonl")
	log, err := changelog.Open(path, nil, signing.Keyring{})
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	wrote(t, log, changelog.SetupSaved, changelog.SkillWritten)

	report, err := verify(t, path, signing.Keyring{})
	if err != nil || report.Events != 2 || report.Unsigned != 2 || report.Signed != 0 {
		t.Fatalf("%+v %v", report, err)
	}
}

// A line signed by a key the verifier does not hold is not a forgery, it is a
// line the verifier cannot attribute yet.
func TestALineSignedByAKeyNotInTheKeyringIsCountedAndNotABreak(t *testing.T) {
	log, path, _ := opened(t)
	wrote(t, log, changelog.SetupSaved, changelog.SkillWritten)

	report, err := verify(t, path, signing.Keyring{})
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, count := range report.Unknown {
		total += count
	}
	if report.Events != 2 || total != 2 || len(report.Signers) != 0 {
		t.Fatalf("%+v", report)
	}
}

func TestManyWritersProduceOneUnbrokenChain(t *testing.T) {
	log, path, keyring := opened(t)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wrote(t, log, changelog.SetupSaved)
		}()
	}
	wg.Wait()
	if report, err := verify(t, path, keyring); err != nil || report.Events != 40 {
		t.Fatalf("%+v %v", report, err)
	}
}

func TestWhatALogLearnedSurvivesBeingOpenedAgain(t *testing.T) {
	log, path, keyring := opened(t)
	log.Append(changelog.Event{Who: "Alice@Example.com", Role: "runner", Kind: changelog.SignedIn})
	log.Append(changelog.Event{Kind: changelog.ServerStarted, What: map[string]string{"GUSTO_ADMINS": "alice@example.com"}})
	if role, found := log.LastRole("alice@example.com"); !found || role != "runner" {
		t.Fatalf("live: %q %v", role, found)
	}
	log.Close()

	again, err := changelog.Open(path, nil, keyring)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if role, found := again.LastRole("ALICE@example.com"); !found || role != "runner" {
		t.Errorf("reopened: %q %v", role, found)
	}
	if _, found := again.LastRole("nobody@example.com"); found {
		t.Error("somebody who never signed in has a role")
	}
	if got := again.LastSettings()["GUSTO_ADMINS"]; got != "alice@example.com" {
		t.Errorf("the settings it last started with: %q", got)
	}
}

func TestAnInboxFileWhoseEventIsInTheLogIsKnownAfterARestart(t *testing.T) {
	log, path, keyring := opened(t)
	if _, err := log.Append(changelog.Event{Who: "ada@acme.com", Kind: changelog.MemberRemoved, What: map[string]string{changelog.InboxKey: "0001-abcd", "email": "bob@acme.com"}}); err != nil {
		t.Fatal(err)
	}
	if !log.Taken("0001-abcd") || log.Taken("0002-ffff") {
		t.Fatal("the log does not know which inbox files it has")
	}
	log.Close()
	reopened, err := changelog.Open(path, nil, keyring)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if !reopened.Taken("0001-abcd") {
		t.Error("a restarted server would write an event twice")
	}
	if reopened.Taken("0002-ffff") {
		t.Error("a file that was never taken is known")
	}
}

func TestOnlyTheChangesTheFrontDoorReportsAreItsToReport(t *testing.T) {
	for _, kind := range []string{changelog.MemberInvited, changelog.MemberRole, changelog.MemberRemoved} {
		if !changelog.FromFrontDoor(kind) {
			t.Errorf("%s is the front door's", kind)
		}
	}
	for _, kind := range []string{changelog.SignedIn, changelog.RoleChanged, changelog.SettingsChanged, changelog.ServerStarted, changelog.TokenCreated, ""} {
		if changelog.FromFrontDoor(kind) {
			t.Errorf("the front door may not report %q", kind)
		}
	}
}
