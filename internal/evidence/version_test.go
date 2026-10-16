package evidence_test

import (
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
)

func manifestIn(t *testing.T, text string) job.Dir {
	t.Helper()
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir.OutFile("manifest.json"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestAManifestWithNoVersionReadsAsVersionOne(t *testing.T) {
	dir := manifestIn(t, `{"job": "abc", "signer": "s", "attestation": "unknown here"}`)
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || manifest.Signer != "s" {
		t.Fatalf("version %d, signer %q", manifest.Version, manifest.Signer)
	}
}

func TestAManifestFromANewerVersionIsRefusedByName(t *testing.T) {
	dir := manifestIn(t, `{"version": 2, "job": "abc"}`)
	_, err := evidence.Read(dir)
	if err == nil || !strings.Contains(err.Error(), "manifest.json is version 2") {
		t.Fatalf("want the version named, got %v", err)
	}
}

func TestAWrittenManifestCarriesItsVersion(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{Job: "abc"}); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(dir.OutFile("manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), `"version": 1`) {
		t.Fatalf("want the version written:\n%s", text)
	}
}
