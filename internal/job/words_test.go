package job

import (
	"slices"
	"testing"
)

func TestAGateKeepsItsQuotedArgumentWhole(t *testing.T) {
	for line, want := range map[string][]string{
		`go test ./...`:           {"go", "test", "./..."},
		`sh -c "test 2 = 2"`:      {"sh", "-c", "test 2 = 2"},
		`sh -c 'echo "hi there"'`: {"sh", "-c", `echo "hi there"`},
		`  spaced   out  `:        {"spaced", "out"},
		``:                        nil,
		`sh -c ""`:                {"sh", "-c", ""},
	} {
		if got := Words(line); !slices.Equal(got, want) {
			t.Errorf("%q split to %#v, want %#v", line, got, want)
		}
	}
}

func TestASensitivityNobodyEnforcesIsRefused(t *testing.T) {
	dir := Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := Spec{Brief: "b", Repo: "r", Gate: []string{"true"}, Harness: "fixture"}
	for label, want := range map[Sensitivity]bool{
		"public": true, "internal": true, "restricted": true,
		"secret": false, "Restricted": false, "confidential": false, "RESTRICTED": false,
	} {
		spec.Sensitivity = label
		if err := spec.Write(dir); err != nil {
			t.Fatal(err)
		}
		_, err := Read(dir)
		if (err == nil) != want {
			t.Errorf("%q: read error %v, want accepted=%t", label, err, want)
		}
	}
}
