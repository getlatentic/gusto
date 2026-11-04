package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// awaitImage is the catalog of a server whose containers could run nothing when it started, while the runner image can
// be fetched. A server kept running across a release starts before its new image is here, and the image arrives only with
// the first container, so every agent would read as not on this server until the server started again. The image is
// fetched in the background and asked again; pages show what the start found until then. A fetch that fails is tried again
// later, since the image may be published after the server started. read is nil when there is nothing to wait for, and done
// closes when the waiting ends: the image is here, or ctx ended.
func awaitImage(ctx context.Context, offered []catalog.Entry, fetch func(context.Context) error, survey func() []catalog.Entry) (read func() []catalog.Entry, done <-chan struct{}) {
	ended := make(chan struct{})
	if slices.ContainsFunc(offered, func(entry catalog.Entry) bool { return entry.Ready }) {
		close(ended)
		return nil, ended
	}
	var current atomic.Pointer[[]catalog.Entry]
	current.Store(&offered)
	go func() {
		defer close(ended)
		for attempt := 0; ; attempt++ {
			err := fetch(ctx)
			if err == nil {
				break
			}
			fmt.Println("the agents stay not offered until the runner image can be fetched:", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(imageRetry(attempt)):
			}
		}
		fresh := survey()
		current.Store(&fresh)
	}()
	return func() []catalog.Entry { return *current.Load() }, ended
}

// imageRetry is how long after a failed fetch of the runner image it is tried again: half a minute, doubling to ten.
var imageRetry = func(attempt int) time.Duration {
	return min(30*time.Second<<min(attempt, 5), 10*time.Minute)
}

// imageFetched says whether this server's containers start from an image it can fetch when it does not have it.
func imageFetched() bool {
	return !runsHere() && os.Getenv("GUSTO_SANDBOX") != "openshell" && os.Getenv(sandbox.ImageSource) != ""
}
