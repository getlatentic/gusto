package ship_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/ship"
	"github.com/getlatentic/gusto/pkg/job"
)

var here = ship.Options{GitHub: "o/r"}

func TestAJobWhoseGateFailedIsRefused(t *testing.T) {
	dir, _ := finished(t, "maybe")
	_, err := ship.Prepare(dir, here)
	if !errors.Is(err, ship.ErrRefused) || !strings.Contains(err.Error(), "did not pass") {
		t.Fatalf("want a refusal naming the gate, got %v", err)
	}
}

func TestEvidenceChangedSinceTheRunIsRefused(t *testing.T) {
	for name, tamper := range map[string]func(t *testing.T, dir job.Dir){
		"a changed patch": func(t *testing.T, dir job.Dir) {
			write(t, dir.OutFile("diff.patch"), "something the gate never saw\n")
		},
		"a file added after the run": func(t *testing.T, dir job.Dir) {
			write(t, dir.OutFile("extra.patch"), "smuggled in\n")
		},
		"a changed job": func(t *testing.T, dir job.Dir) {
			write(t, dir.Spec(), `{"brief": "something else", "gate": ["true"], "harness": "yes"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, _ := finished(t, "yes")
			tamper(t, dir)
			_, err := ship.Prepare(dir, here)
			if !errors.Is(err, ship.ErrRefused) || !strings.Contains(err.Error(), "changed since the run") {
				t.Fatalf("want a refusal naming the evidence, got %v", err)
			}
		})
	}
}

// The manifest does not hash itself, so rewriting its verdict leaves every hash
// intact. gates.json still holds the gate's real answer.
func TestAManifestRewrittenToClaimAPassIsRefused(t *testing.T) {
	dir, _ := finished(t, "maybe")
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Gate.Passed, manifest.Gate.Code = true, 0
	if err := evidence.Write(dir, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := ship.Prepare(dir, here); !errors.Is(err, ship.ErrRefused) {
		t.Fatalf("a rewritten manifest must not ship, got %v", err)
	}
}

func TestADryRunTouchesNothing(t *testing.T) {
	dir, bare := finished(t, "yes")
	plan, err := ship.Prepare(dir, here)
	if err != nil {
		t.Fatal(err)
	}
	fake := &forge{}
	server := fake.serve(t)
	refs, files := git(t, bare, "for-each-ref"), snapshot(t, dir)
	var out bytes.Buffer
	shipper := ship.Shipper{Pusher: ship.Git{}, Forge: ship.GitHub{API: server.URL}, Out: &out, DryRun: true}
	if _, err := shipper.Ship(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if after := git(t, bare, "for-each-ref"); after != refs {
		t.Fatalf("a dry run changed the remote:\n%s\n%s", refs, after)
	}
	if after := snapshot(t, dir); after != files {
		t.Fatal("a dry run changed the job directory")
	}
	if len(fake.requests()) != 0 {
		t.Fatalf("a dry run called the forge: %v", fake.requests())
	}
	for _, line := range []string{"dry run", plan.Branch, plan.At, "into main"} {
		if !strings.Contains(out.String(), line) {
			t.Fatalf("the dry run should say %q:\n%s", line, out.String())
		}
	}
}

func TestTheBranchCarriesThePatchAtTheCommitTheJobRanAgainst(t *testing.T) {
	dir, bare := finished(t, "yes")
	plan, address := shipped(t, dir, &forge{})
	if address != "https://github.com/o/r/pull/1" {
		t.Fatalf("want the pull request's address, got %q", address)
	}
	if answer := git(t, bare, "show", plan.Branch+":answer.txt"); answer != "yes" {
		t.Fatalf("the branch should carry the patch, answer.txt says %q", answer)
	}
	if parent := git(t, bare, "rev-parse", plan.Branch+"^"); parent != plan.At {
		t.Fatalf("the branch should start at %s, starts at %s", plan.At, parent)
	}
	if main := git(t, bare, "rev-parse", "main"); main != plan.At {
		t.Fatal("shipping must leave the base branch where it was")
	}
}

func TestThePullRequestNamesTheHarnessTheModelTheGateAndTheManifest(t *testing.T) {
	dir, _ := finished(t, "yes")
	fake := &forge{}
	plan, _ := shipped(t, dir, fake)
	requests := fake.requests()
	if len(requests) != 1 {
		t.Fatalf("want one request, got %d", len(requests))
	}
	sent := requests[0].Body
	if sent["head"] != plan.Branch || sent["base"] != "main" {
		t.Fatalf("head %v, base %v", sent["head"], sent["base"])
	}
	body, _ := sent["body"].(string)
	for _, named := range []string{"`yes`", "`local/fixture-model`", `sh -c 'test "$(cat answer.txt)" = yes'`, plan.Manifest, plan.At} {
		if !strings.Contains(body, named) {
			t.Fatalf("the body should name %q:\n%s", named, body)
		}
	}
}

func TestShippingTheSameRunTwiceMakesTheSameBranch(t *testing.T) {
	dir, bare := finished(t, "yes")
	first, _ := shipped(t, dir, &forge{})
	tip := git(t, bare, "rev-parse", first.Branch)
	second, _ := shipped(t, dir, &forge{})
	if second.Branch != first.Branch || git(t, bare, "rev-parse", second.Branch) != tip {
		t.Fatalf("the same run should ship as the same commit on the same branch: %s, %s", first.Branch, second.Branch)
	}
	if !strings.HasPrefix(first.Branch, "gusto/20260926-0900-make-the-answer-yes-") {
		t.Fatalf("the branch should be named for the job: %s", first.Branch)
	}
}

func shipped(t *testing.T, dir job.Dir, fake *forge) (ship.Plan, string) {
	t.Helper()
	plan, err := ship.Prepare(dir, here)
	if err != nil {
		t.Fatal(err)
	}
	server := fake.serve(t)
	shipper := ship.Shipper{Pusher: ship.Git{}, Forge: ship.GitHub{API: server.URL, Token: "t"}, Out: &bytes.Buffer{}}
	address, err := shipper.Ship(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	return plan, address
}

// snapshot is every path and its contents under the job directory, workspace
// and .git included, so any write at all shows up.
func snapshot(t *testing.T, dir job.Dir) string {
	t.Helper()
	var out strings.Builder
	err := filepath.WalkDir(dir.Path(), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		text, err := os.ReadFile(path)
		out.WriteString(path + "\n" + string(text) + "\n")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out.String()
}
