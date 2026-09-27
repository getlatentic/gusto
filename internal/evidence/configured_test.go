package evidence_test

import (
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
)

const (
	oneDigest   = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	otherDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

func TestAManifestFromBeforeTheDigestSaysItHasNone(t *testing.T) {
	dir := manifestIn(t, `{"job": "abc", "harness": "opencode", "signer": "s"}`)
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.ConfiguredHarness(); got != evidence.NoDigest {
		t.Fatalf("got %q, want %q", got, evidence.NoDigest)
	}
}

func TestWhetherTwoRunsAreComparableAndWhy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		a, b       evidence.Manifest
		comparable bool
		because    string
	}{
		{"the same digest",
			evidence.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			evidence.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			true, "both ran opencode as " + oneDigest},
		{"the same name, configured differently",
			evidence.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			evidence.Manifest{Harness: "opencode", HarnessDigest: otherDigest},
			false, "both are called opencode but were configured differently"},
		{"the same name and no digest",
			evidence.Manifest{Harness: "opencode"},
			evidence.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			false, "opencode is " + evidence.NoDigest},
		{"different harnesses",
			evidence.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			evidence.Manifest{Harness: "claude", HarnessDigest: otherDigest},
			false, "they ran different harnesses"},
	} {
		comparable, because := evidence.Comparable(tc.a, tc.b)
		if comparable != tc.comparable || !strings.Contains(because, tc.because) {
			t.Errorf("%s: got %v, %q; want %v, %q", tc.name, comparable, because, tc.comparable, tc.because)
		}
	}
}
