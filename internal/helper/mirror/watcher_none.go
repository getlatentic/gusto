//go:build !(linux || windows || (darwin && cgo))

package mirror

import "context"

func watch(context.Context, string, *Ignore, func(string)) (<-chan struct{}, error) {
	return nil, ErrNoWatcher
}
