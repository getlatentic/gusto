package mirror

import (
	"context"
	"time"
)

// Keep keeps the folders in step until the context ends: it runs once, waits, and runs again. The wait is short while there is
// something going on, which is a run that did something or failed, and longer when nothing has changed in a while, so an idle
// folder costs a listing every half minute and a busy one every few seconds.
func (e *Engine) Keep(ctx context.Context, quick, slow time.Duration, told func(*Report, error)) error {
	wait := quick
	for ctx.Err() == nil {
		report, err := e.Once(ctx)
		if told != nil {
			told(report, err)
		}
		busy := err != nil
		if report != nil {
			for kind, n := range report.Done {
				if kind != Settle && kind != Forget && n > 0 {
					busy = true
				}
			}
			busy = busy || len(report.Errors) > 0
		}
		if busy {
			wait = quick
		} else if wait < slow {
			wait = min(wait*2, slow)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
	}
	return nil
}
