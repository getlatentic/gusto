package mirror

import (
	"context"
	"errors"
)

// ErrNoWatcher is a build that cannot watch a folder: the timed runs keep it.
var ErrNoWatcher = errors.New("this build cannot watch a folder for changes")

// Watch sends on the channel when something under root may have changed, until the context ends. Wakes that come while one is
// waiting are one wake. It says what it can do about a limit it reaches through told, and goes on with what it has.
func Watch(ctx context.Context, root string, ignore *Ignore, told func(string)) (<-chan struct{}, error) {
	return watch(ctx, root, ignore, told)
}

// pulse makes a wake that is not lost and not piled up.
func pulse(wake chan<- struct{}) {
	select {
	case wake <- struct{}{}:
	default:
	}
}
