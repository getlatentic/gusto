package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/computer"
	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/quota"
)

// turnBy is a turn in a conversation on an instance, asked for by a person, run by the work.
func turnBy(t *testing.T, work jobs.Work, index *jobs.Index, instances *instance.Store, held instance.Instance, threadID, who, brief string) job.Dir {
	t.Helper()
	id, err := index.Reserve(brief, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	remembered := instances.Latest(held.ID).Remembered
	record := jobs.Record{ID: id, State: jobs.Running, Brief: brief, Harness: "fixture", Asked: who, Instance: held.ID, Thread: threadID, Queued: jobs.Now()}
	dir := record.Dir(index.Root())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: brief, Harness: "fixture", Session: held.ID, Thread: threadID, Remembered: remembered}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := index.Add(record); err != nil {
		t.Fatal(err)
	}
	if _, err := work(dir); err != nil {
		t.Logf("%s: %v", who, err)
	}
	return dir
}

// An agent that answers as jsonl, says what it calls the conversation, and adds a line to the
// notes it finds in the workspace.
func conversational(t *testing.T) {
	t.Helper()
	harnesses := t.TempDir()
	os.WriteFile(filepath.Join(harnesses, "fixture.json"), []byte(`{"name":"fixture","program":"sh",
	  "argv":["-c","echo '{\"sessionID\":\"s-42\"}'; echo more >> notes.txt; pwd | sed 's|.*/sandbox||' > where.txt"],
	  "needs":"nothing","stream":"jsonl","sessionId":"sessionID"}`), 0o644)
	t.Setenv("HAAS_HARNESSES", harnesses)
}

func TestATurnWorksOnTheInstancesFilesInThePersonsComputerAndTheNextStartsFromWhatItLeft(t *testing.T) {
	work, index, registry, _, _, instances := organisationWithInstances(t, quota.Quota{Computers: 4, Jobs: 100})
	conversational(t)
	held, err := instances.Start("20260929-1200-notes", instance.Instance{Title: "Notes", Harness: "fixture", Who: "ada@acme.com"})
	if err != nil {
		t.Fatal(err)
	}
	thread := instances.Latest(held.ID)
	os.WriteFile(filepath.Join(instances.Workspace(held.ID), "notes.txt"), []byte("first\n"), 0o644)

	one := turnBy(t, work, index, instances, held, thread.ID, "ada@acme.com", "the first turn")
	notes, _ := os.ReadFile(filepath.Join(instances.Workspace(held.ID), "notes.txt"))
	if string(notes) != "first\nmore\n" {
		t.Fatalf("the turn worked on the instance's file and the change came back: %q", notes)
	}
	if where, _ := os.ReadFile(filepath.Join(instances.Workspace(held.ID), "where.txt")); !strings.HasPrefix(string(where), "/jobs/") {
		t.Errorf("the turn ran in the person's computer: %q", where)
	}
	if got, err := registry.Of("ada@acme.com"); err != nil || got.State != computer.Running {
		t.Errorf("%+v %v", got, err)
	}
	if manifest, err := evidence.Read(one); err != nil || manifest.Session != "s-42" {
		t.Fatalf("the record names the conversation: %+v %v", manifest, err)
	}
	if latest := instances.Latest(held.ID); latest.Remembered != "s-42" {
		t.Errorf("the conversation is learned, so the next turn resumes it: %+v", latest)
	}

	turnBy(t, work, index, instances, held, thread.ID, "ada@acme.com", "the second turn")
	notes, _ = os.ReadFile(filepath.Join(instances.Workspace(held.ID), "notes.txt"))
	if string(notes) != "first\nmore\nmore\n" {
		t.Errorf("the second turn started from what the first left: %q", notes)
	}
}

// A turn that changed files and failed before its record was written leaves the instance's
// files as they were.
func TestATurnThatFailsLeavesTheInstancesFilesAsTheyWere(t *testing.T) {
	work, index, _, _, _, instances := organisationWithInstances(t, quota.Quota{Computers: 4, Jobs: 100})
	conversational(t)
	held, _ := instances.Start("20260929-1200-notes", instance.Instance{Title: "Notes", Harness: "fixture", Who: "ada@acme.com"})
	thread := instances.Latest(held.ID)
	os.WriteFile(filepath.Join(instances.Workspace(held.ID), "notes.txt"), []byte("precious\n"), 0o644)

	failGuard(t)
	turnBy(t, work, index, instances, held, thread.ID, "ada@acme.com", "a turn that fails")
	notes, _ := os.ReadFile(filepath.Join(instances.Workspace(held.ID), "notes.txt"))
	if string(notes) != "precious\n" {
		t.Errorf("a failed turn changed the instance: %q", notes)
	}
	if latest := instances.Latest(held.ID); latest.Remembered != "" {
		t.Errorf("a failed turn learned a conversation: %+v", latest)
	}
}

// failGuard makes the gateway answer for the log once, when a turn starts, and refuse after it,
// which a turn finds out
// after its workspace is out of the sandbox and before its record is written.
func failGuard(t *testing.T) {
	t.Helper()
	path, err := exec.LookPath("openshell")
	if err != nil {
		t.Fatal(err)
	}
	script, _ := os.ReadFile(path)
	broken := strings.Replace(string(script), `"logs "*|"logs") echo '{"event":"net.allow"}';;`, `"logs "*|"logs") [ -e "$root/logged" ] && { echo "Error: no logs" >&2; exit 1; }; touch "$root/logged"; echo '{"event":"net.allow"}';;`, 1)
	if broken == string(script) {
		t.Fatal("the gateway fake has no logs answer to break")
	}
	os.WriteFile(path, []byte(broken), 0o755)
}

func TestATreeIsCopiedWithItsLinksAsLinksAndNeverFollowedOutOfIt(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("not the agent's\n"), 0o600)
	from := filepath.Join(t.TempDir(), "workspace")
	os.MkdirAll(filepath.Join(from, "sub", "deep"), 0o755)
	os.WriteFile(filepath.Join(from, "a.txt"), []byte("a\n"), 0o644)
	os.WriteFile(filepath.Join(from, "sub", "deep", "run.sh"), []byte("#!/bin/sh\n"), 0o755)
	os.Symlink(outside, filepath.Join(from, "link-to-secret"))
	os.Symlink("a.txt", filepath.Join(from, "link-to-a"))
	to := filepath.Join(t.TempDir(), "copy")
	os.MkdirAll(to, 0o755)
	os.WriteFile(filepath.Join(to, "stale.txt"), []byte("old\n"), 0o644)

	if err := replaceTree(from, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(to, "stale.txt")); err == nil {
		t.Error("what was there is replaced and not merged")
	}
	if info, err := os.Lstat(filepath.Join(to, "link-to-secret")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("a link is a link, and its target is not copied in: %v %v", info, err)
	}
	if target, _ := os.Readlink(filepath.Join(to, "link-to-a")); target != "a.txt" {
		t.Errorf("%q", target)
	}
	if info, err := os.Stat(filepath.Join(to, "sub", "deep", "run.sh")); err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("a file keeps its mode: %v %v", info, err)
	}
	if got, _ := os.ReadFile(filepath.Join(to, "a.txt")); string(got) != "a\n" {
		t.Errorf("%q", got)
	}
	for _, leftover := range []string{to + ".next", to + ".old"} {
		if _, err := os.Stat(leftover); err == nil {
			t.Errorf("%s was left behind", leftover)
		}
	}
}

func TestACopyThatFailsLeavesWhatWasThere(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads what a mode of 0 forbids")
	}
	from := filepath.Join(t.TempDir(), "workspace")
	os.MkdirAll(from, 0o755)
	os.WriteFile(filepath.Join(from, "fine.txt"), []byte("fine\n"), 0o644)
	os.WriteFile(filepath.Join(from, "unreadable.txt"), []byte("no\n"), 0o000)
	to := filepath.Join(t.TempDir(), "copy")
	os.MkdirAll(to, 0o755)
	os.WriteFile(filepath.Join(to, "kept.txt"), []byte("kept\n"), 0o644)
	if err := replaceTree(from, to); err == nil {
		t.Fatal("a file that cannot be read fails the copy")
	}
	if got, _ := os.ReadFile(filepath.Join(to, "kept.txt")); string(got) != "kept\n" {
		t.Errorf("what was there is left: %q", got)
	}
	if _, err := os.Stat(to + ".next"); err == nil {
		t.Error("a half-made copy was left behind")
	}
}
