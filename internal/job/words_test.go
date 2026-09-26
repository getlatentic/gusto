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
