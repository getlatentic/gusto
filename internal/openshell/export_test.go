package openshell

import "time"

// SetSettle lets a test wait milliseconds where a run waits seconds.
func SetSettle(wait time.Duration, rounds int) { settle, settleRounds = wait, rounds }
