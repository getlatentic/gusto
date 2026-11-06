package container

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
)

// awaitImage fetches the runner image for a server whose containers could run nothing when it started. A server kept running
// across a release starts before its new image is here, and the image arrives only with the first container, so every agent
// would read as not on this server until the server started again. Once the image is here it is asked again, and what it carries
// is kept. A fetch that fails is tried again after retry(attempt), since the image may be published after the server started.
// done closes when the waiting ends: the image is here, or ctx ended.
func awaitImage(ctx context.Context, offered []catalog.Entry, fetch func(context.Context) error, survey func() []catalog.Entry,
	keep func([]catalog.Entry), retry func(attempt int) time.Duration) (done <-chan struct{}) {
	ended := make(chan struct{})
	if slices.ContainsFunc(offered, func(entry catalog.Entry) bool { return entry.Ready }) {
		close(ended)
		return ended
	}
	go func() {
		defer close(ended)
		for attempt := 0; ; attempt++ {
			err := fetch(ctx)
			if err == nil {
				break
			}
			slog.Warn("the agents stay not offered until the runner image can be fetched", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retry(attempt)):
			}
		}
		keep(survey())
	}()
	return ended
}

// imageRetry is how long after a failed fetch of the runner image it is tried again: half a minute, doubling to ten.
func imageRetry(attempt int) time.Duration {
	return min(30*time.Second<<min(attempt, 5), 10*time.Minute)
}
