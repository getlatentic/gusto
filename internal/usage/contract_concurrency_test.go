package usage_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/usage"
)

var concurrencyCases = []contractCase{
	{"many goroutines add and list at once", func(t *testing.T, s usage.Store) {
		const writers, batches, perBatch = 4, 6, 25
		var wg sync.WaitGroup
		errs := make(chan error, 64)
		for w := 0; w < writers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for b := 0; b < batches; b++ {
					rows := make([]usage.Row, perBatch)
					for i := range rows {
						rows[i] = callRow(fmt.Sprintf("w%d", w), b*perBatch+i, fmt.Sprintf("s-%d-%d", w, i%5), b*perBatch+i)
					}
					if err := s.Add(ctx, rows); err != nil {
						errs <- err
					}
				}
			}()
		}
		for r := 0; r < 4; r++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 8; i++ {
					if _, _, err := s.Sessions(ctx, usage.Filter{Search: "s-"}, usage.Page{Limit: 10}); err != nil {
						errs <- err
					}
					if _, err := s.Totals(ctx, usage.Filter{}); err != nil {
						errs <- err
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}
		if got := totals(t, s, usage.Filter{}).Calls; got != writers*batches*perBatch {
			t.Fatalf("calls = %d", got)
		}
	}},
}
