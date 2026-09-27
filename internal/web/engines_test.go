package web

import (
	"testing"

	"github.com/getlatentic/haas/internal/catalog"
)

// Each engine is a row and each model a column, and a pair nobody ran is
// shown as not tried rather than as a failure.
func TestTheMeasuredGridKeepsUntriedApartFromFailed(t *testing.T) {
	seconds, cost := 10.4, 0.0014
	view := measuredOf([]catalog.Entry{
		{Title: "Codex", Measured: []catalog.Measured{
			{Model: "deepseek/deepseek-flash", Finished: true, WallSeconds: &seconds, CostUSD: &cost},
			{Model: "deepseek/deepseek-v4-pro", Finished: false},
		}},
		{Title: "Pi", Measured: []catalog.Measured{{Model: "anthropic/deepseek-flash", Finished: true}}},
		{Title: "Unmeasured"},
	})
	if len(view.Rows) != 2 || len(view.Models) != 2 {
		t.Fatalf("want two engines by two models, got %+v", view)
	}
	if view.Finished != 2 || view.Tried != 3 {
		t.Errorf("finished %d of %d, want 2 of 3", view.Finished, view.Tried)
	}
	codex, pi := view.Rows[0].Cells, view.Rows[1].Cells
	if !codex[0].Finished || codex[0].Took != "10 s" || codex[0].Cost != "$0.0014" {
		t.Errorf("codex on flash = %+v", codex[0])
	}
	if !codex[1].Tried || codex[1].Finished {
		t.Errorf("codex on pro was tried and did not finish, got %+v", codex[1])
	}
	if pi[1].Tried {
		t.Errorf("pi was never run on pro, got %+v", pi[1])
	}
}
