package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/getlatentic/haas/internal/helper"
	"github.com/getlatentic/haas/internal/helper/mirror"
)

// backstop is how often the folders are compared with nothing to say that they changed. A watcher can drop events, and a change
// it missed still crosses by this.
const backstop = 5 * time.Minute

// wakes is the channel that tells the sync something may have changed: a save in the folder, and any word of the agent's turns,
// since the end of a turn is when its files have just changed. With neither it is nil and the timed runs keep the folders.
func wakes(ctx context.Context, root string, ignore *mirror.Ignore, client helper.Client, agent string) (<-chan struct{}, time.Duration) {
	slow := 30 * time.Second
	merged := make(chan struct{}, 1)
	send := func() {
		select {
		case merged <- struct{}{}:
		default:
		}
	}
	if saves, err := mirror.Watch(ctx, root, ignore, func(advice string) { fmt.Fprintln(os.Stderr, advice) }); err == nil {
		slow = backstop
		go forward(ctx, saves, send)
	} else if !errors.Is(err, mirror.ErrNoWatcher) {
		fmt.Fprintf(os.Stderr, "the folder is not watched (%v): it is compared every %s\n", err, slow)
	}
	hub := helper.NewHub()
	turns, stop := hub.Listen()
	go follow(ctx, client, hub)
	go func() {
		defer stop()
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-turns:
				if event.Turn.Instance == agent {
					send()
				}
			}
		}
	}()
	return merged, slow
}

func forward(ctx context.Context, from <-chan struct{}, send func()) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-from:
			send()
		}
	}
}

// follow holds the event stream open for as long as the context lasts, and gets it back after it is lost.
func follow(ctx context.Context, client helper.Client, hub *helper.Hub) {
	for attempt := 0; ctx.Err() == nil; attempt++ {
		err := client.Follow(ctx, hub, func(up bool) {
			if up {
				attempt = -1
			}
		})
		if errors.Is(err, helper.ErrSignedOut) {
			return
		}
		select {
		case <-ctx.Done():
		case <-time.After(helper.Backoff(max(attempt, 0))):
		}
	}
}
