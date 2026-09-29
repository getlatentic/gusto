package repoaccess

import "time"

// AllowHTTP lets a test clone from a forge it runs on plain http. It returns
// what puts the rule back.
func AllowHTTP() func() {
	was := protocols
	protocols = "GIT_ALLOW_PROTOCOL=https:ssh:file:http"
	return func() { protocols = was }
}

// SetRetryDelay lets a test wait milliseconds where a clone waits seconds. It
// returns what puts the delay back.
func SetRetryDelay(d time.Duration) func() {
	was := retryDelay
	retryDelay = d
	return func() { retryDelay = was }
}
