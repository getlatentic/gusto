package rules

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/job"
)

func spec(label job.Sensitivity) job.Spec {
	return job.Spec{Brief: "b", Repo: "r", Gate: []string{"true"}, Harness: "opencode", Sensitivity: label}
}

func TestRestrictedCodeRefusesAModelOnTheInternet(t *testing.T) {
	err := Allows(spec(job.Restricted), "https://api.example.com/v1")
	if err == nil || !strings.Contains(err.Error(), "api.example.com") {
		t.Fatalf("want a refusal naming the endpoint, got %v", err)
	}
}

func TestRestrictedCodeAcceptsAModelInTheBuilding(t *testing.T) {
	for _, endpoint := range []string{
		"http://localhost:11434/v1",
		"http://host.docker.internal:11434/v1",
		"http://127.0.0.1:8000",
		"http://10.1.2.3:8000/v1",
		"http://172.20.0.5:8000",
		"http://gpu.internal/v1",
	} {
		if err := Allows(spec(job.Restricted), endpoint); err != nil {
			t.Errorf("%s: %v", endpoint, err)
		}
	}
}

func TestAPublicLookingAddressIsNotPrivate(t *testing.T) {
	for _, endpoint := range []string{"http://172.15.0.1/v1", "http://172.32.0.1/v1", "http://11.0.0.1/v1"} {
		if err := Allows(spec(job.Restricted), endpoint); err == nil {
			t.Errorf("%s reads as local", endpoint)
		}
	}
}

func TestInternalCodeMayUseAVendorModel(t *testing.T) {
	if err := Allows(spec(job.Internal), "https://api.example.com/v1"); err != nil {
		t.Fatal(err)
	}
}

func TestAHarnessWithNoModelIsAllowedWhateverTheLabel(t *testing.T) {
	// A harness can be a program that makes the change. Nothing leaves, so the
	// restricted label has nothing to refuse.
	if err := Allows(spec(job.Restricted), ""); err != nil {
		t.Fatal(err)
	}
}
