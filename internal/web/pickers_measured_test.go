package web

import (
	"testing"

	"github.com/getlatentic/gusto/internal/catalog"
)

func TestAModelSaysWhatTheBenchmarkSawTheAgentDoWithIt(t *testing.T) {
	seconds := 27.0
	entry := catalog.Entry{Name: "pi", Measured: []catalog.Measured{
		{Model: "gemma/fast", Job: "one-line-fix", Finished: true, WallSeconds: &seconds},
		{Model: "gemma/tiny", Job: "one-line-fix", Finished: false},
	}}
	for model, want := range map[string]string{
		"gemma/fast":  "finished our test in 27s",
		"gemma/tiny":  "did not finish our test",
		"gemma/other": "",
	} {
		if got := measuredWords(entry, model); got != want {
			t.Errorf("%s: %q, want %q", model, got, want)
		}
	}
	if got := joinDetail("$0.13 in, $0.4 out per 1M", "finished our test in 27s"); got != "$0.13 in, $0.4 out per 1M · finished our test in 27s" {
		t.Errorf("%q", got)
	}
}
