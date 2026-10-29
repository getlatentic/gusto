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

// The new agent dialog offers every agent type that can run, and says under each what the benchmark saw it do with the models
// offered here: its quickest finish, or that it finished on none of those it was tried with.
func TestEachAgentTypeSaysHowItDidOnTheModelsOfferedHere(t *testing.T) {
	quick, slow := 27.0, 41.0
	entries := []catalog.Entry{
		{Name: "pi", Measured: []catalog.Measured{
			{Model: "gemma/big", Finished: true, WallSeconds: &slow},
			{Model: "gemma/fast", Finished: true, WallSeconds: &quick},
			{Model: "elsewhere", Finished: true, WallSeconds: &quick},
		}},
		{Name: "dsh", Measured: []catalog.Measured{{Model: "gemma/big", Finished: false}, {Model: "gemma/fast", Finished: false}, {Model: "elsewhere", Finished: true}}},
		{Name: "codex"},
	}
	kinds := kindsFor(entries, []string{"gemma/big", "gemma/fast"})
	want := []struct {
		words  string
		failed bool
	}{{"Finished our test in 27s", false}, {"Did not finish our test on the models here", true}, {"", false}}
	for i, each := range want {
		if kinds[i].Measured != each.words || kinds[i].Failed != each.failed || kinds[i].Name != entries[i].Name {
			t.Errorf("%s: %q %v, want %q %v", entries[i].Name, kinds[i].Measured, kinds[i].Failed, each.words, each.failed)
		}
	}
}
