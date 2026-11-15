package container

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
)

func soon(int) time.Duration { return time.Millisecond }

// kept is where awaitImage keeps what it found, read as the backend reads it.
type kept struct {
	entries atomic.Pointer[[]catalog.Entry]
}

func (k *kept) keep(fresh []catalog.Entry) { k.entries.Store(&fresh) }

func (k *kept) read() []catalog.Entry {
	if found := k.entries.Load(); found != nil {
		return *found
	}
	return nil
}

// A server that started before its image was here offers the agents once the image is fetched, without starting again.
func TestTheCatalogIsAskedAgainOnceTheImageIsFetched(t *testing.T) {
	missing := []catalog.Entry{{Name: "claude", Why: "the runner image could not be asked what it carries"}}
	fetched := make(chan struct{})
	var found kept
	done := awaitImage(context.Background(), missing, func(context.Context) error { <-fetched; return nil },
		func() []catalog.Entry { return []catalog.Entry{{Name: "claude", Ready: true}} }, found.keep, soon)
	if found.read() != nil {
		t.Fatal("the catalog offered an agent before the image was here")
	}
	close(fetched)
	<-done
	if got := found.read(); len(got) != 1 || !got[0].Ready {
		t.Errorf("the catalog was not asked again once the image was fetched: %+v", got)
	}
}

// A catalog that offers an agent waits for nothing, and an image that cannot be fetched leaves what the start found.
func TestTheCatalogWaitsOnlyWhenNothingRuns(t *testing.T) {
	var found kept
	<-awaitImage(context.Background(), []catalog.Entry{{Name: "claude", Ready: true}}, nil, nil, found.keep, soon)
	asked := false
	ctx, stop := context.WithCancel(context.Background())
	done := awaitImage(ctx, []catalog.Entry{{Name: "claude"}}, func(context.Context) error { return errors.New("no image") },
		func() []catalog.Entry { asked = true; return nil }, found.keep, soon)
	stop()
	<-done
	if asked || found.read() != nil {
		t.Error("a failed fetch changed the catalog")
	}
}

// A fetch that fails is tried again, so an image published after the server started is offered without a restart.
func TestAFailedImageFetchIsTriedAgainUntilTheImageIsHere(t *testing.T) {
	var tries atomic.Int32
	var found kept
	done := awaitImage(context.Background(), []catalog.Entry{{Name: "claude"}}, func(context.Context) error {
		if tries.Add(1) < 3 {
			return errors.New("not published yet")
		}
		return nil
	}, func() []catalog.Entry { return []catalog.Entry{{Name: "claude", Ready: true}} }, found.keep, soon)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a fetch that failed was not tried again")
	}
	if got := found.read(); len(got) != 1 || !got[0].Ready || tries.Load() != 3 {
		t.Errorf("after %d tries the catalog offers %+v", tries.Load(), got)
	}
}

// The wait after a failed fetch doubles from half a minute and stops growing at ten.
func TestAFailedFetchWaitsLongerEachTimeUpToTenMinutes(t *testing.T) {
	for attempt, want := range map[int]time.Duration{0: 30 * time.Second, 1: time.Minute, 4: 8 * time.Minute, 9: 10 * time.Minute} {
		if got := imageRetry(attempt); got != want {
			t.Errorf("attempt %d waits %s, want %s", attempt, got, want)
		}
	}
}
