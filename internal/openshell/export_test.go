package openshell

import "time"

// SetSettle lets a test wait milliseconds where a run waits seconds.
func SetSettle(wait time.Duration, rounds int) { settle, settleRounds = wait, rounds }

// InSandbox is inSandbox, for the tests.
func InSandbox(text string) string { return inSandbox(text) }

// PlaceConfig is placeConfig, for the tests.
func PlaceConfig(from, home string) string { return placeConfig(from, home) }

// SetRetry lets a test wait milliseconds where a run waits seconds.
func SetRetry(total, every time.Duration) { retryFor, retryEvery = total, every }
