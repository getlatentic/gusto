package web

import (
	"testing"

	"github.com/getlatentic/gusto/internal/catalog"
)

// An agent nobody chose a model for starts on one it finished the simplest task with, and never on one it failed, so DeepSeek
// Agent on a deployment whose default is a model it cannot finish on starts on another (planning#674).
func TestAnAgentStartsOnAModelItFinishedOnAndNotOneItFailed(t *testing.T) {
	dsh := pick{Entry: catalog.Entry{Name: "dsh", Measured: []catalog.Measured{
		{Model: "gemma/26b", Finished: false}, {Model: "deepseek/flash", Finished: true},
	}}, Models: []string{"gemma/26b", "gemma/31b", "deepseek/flash"}}
	if got := startingModel(dsh); got != "deepseek/flash" {
		t.Errorf("it starts on %q", got)
	}
	untried := pick{Entry: catalog.Entry{Name: "dsh", Measured: []catalog.Measured{{Model: "gemma/26b", Finished: false}}},
		Models: []string{"gemma/26b", "gemma/31b"}}
	if got := startingModel(untried); got != "gemma/31b" {
		t.Errorf("it starts on %q, a model it failed on, when one it was never tried on is there", got)
	}
	if got := startingModel(pick{Entry: catalog.Entry{Name: "pi"}, Models: []string{"a", "b"}}); got != "a" {
		t.Errorf("with nothing measured it starts on %q", got)
	}
	if harness, model := defaultPick([]pick{dsh}, nil, chatDefault{Harness: "dsh", Model: "gemma/huge"}); harness != "dsh" || model != "deepseek/flash" {
		t.Errorf("a new chat on dsh starts on %q", model)
	}
}
