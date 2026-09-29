package repoaccess

import "time"

// SetRetryDelay lets a test wait milliseconds where a clone waits seconds. It
// returns what puts the delay back.
func SetRetryDelay(d time.Duration) func() {
	was := retryDelay
	retryDelay = d
	return func() { retryDelay = was }
}
