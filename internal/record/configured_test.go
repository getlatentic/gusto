package record_test

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/record"
)

const (
	oneDigest   = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	otherDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

func TestAManifestFromBeforeTheDigestSaysItHasNone(t *testing.T) {
	dir := manifestIn(t, `{"job": "abc", "harness": "opencode", "signer": "s"}`)
	manifest, err := record.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.ConfiguredHarness(); got != record.NoDigest {
		t.Fatalf("got %q, want %q", got, record.NoDigest)
	}
}

func TestWhetherTwoRunsAreComparableAndWhy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		a, b       record.Manifest
		comparable bool
		because    string
	}{
		{"the same digest",
			record.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			record.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			true, "both ran opencode as " + oneDigest},
		{"the same digest, built differently",
			record.Manifest{Harness: "opencode", HarnessDigest: oneDigest, HarnessVersion: "opencode-ai@1.19.3"},
			record.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			false, "built as opencode-ai@1.19.3 and no build recorded"},
		{"the same name, configured differently",
			record.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			record.Manifest{Harness: "opencode", HarnessDigest: otherDigest},
			false, "both are called opencode but were configured differently"},
		{"the same name and no digest",
			record.Manifest{Harness: "opencode"},
			record.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			false, "opencode is " + record.NoDigest},
		{"different harnesses",
			record.Manifest{Harness: "opencode", HarnessDigest: oneDigest},
			record.Manifest{Harness: "claude", HarnessDigest: otherDigest},
			false, "they ran different agents"},
	} {
		comparable, because := record.Comparable(tc.a, tc.b)
		if comparable != tc.comparable || !strings.Contains(because, tc.because) {
			t.Errorf("%s: got %v, %q; want %v, %q", tc.name, comparable, because, tc.comparable, tc.because)
		}
	}
}
