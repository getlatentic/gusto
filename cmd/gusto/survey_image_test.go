package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
)

// A server that started before its image was here offers the agents once the image is fetched, without starting again.
func TestTheCatalogIsAskedAgainOnceTheImageIsFetched(t *testing.T) {
	missing := []catalog.Entry{{Name: "claude", Why: "the runner image could not be asked what it carries"}}
	fetched := make(chan struct{})
	read, done := awaitImage(context.Background(), missing, func(context.Context) error { <-fetched; return nil },
		func() []catalog.Entry { return []catalog.Entry{{Name: "claude", Ready: true}} })
	if read == nil || read()[0].Ready {
		t.Fatal("the catalog offered an agent before the image was here")
	}
	close(fetched)
	<-done
	if !read()[0].Ready {
		t.Error("the catalog was not asked again once the image was fetched")
	}
}

// A catalog that offers an agent waits for nothing, and an image that cannot be fetched leaves what the start found.
func TestTheCatalogWaitsOnlyWhenNothingRuns(t *testing.T) {
	if read, done := awaitImage(context.Background(), []catalog.Entry{{Name: "claude", Ready: true}}, nil, nil); read != nil {
		t.Error("a catalog that offers an agent waited for the image")
	} else {
		<-done
	}
	asked := false
	ctx, stop := context.WithCancel(context.Background())
	read, done := awaitImage(ctx, []catalog.Entry{{Name: "claude"}}, func(context.Context) error { return errors.New("no image") },
		func() []catalog.Entry { asked = true; return nil })
	stop()
	<-done
	if asked || read()[0].Name != "claude" || read()[0].Ready {
		t.Error("a failed fetch changed the catalog")
	}
}

// A fetch that fails is tried again, so an image published after the server started is offered without a restart.
func TestAFailedImageFetchIsTriedAgainUntilTheImageIsHere(t *testing.T) {
	was := imageRetry
	imageRetry = func(int) time.Duration { return time.Millisecond }
	t.Cleanup(func() { imageRetry = was })
	var tries atomic.Int32
	read, done := awaitImage(context.Background(), []catalog.Entry{{Name: "claude"}}, func(context.Context) error {
		if tries.Add(1) < 3 {
			return errors.New("not published yet")
		}
		return nil
	}, func() []catalog.Entry { return []catalog.Entry{{Name: "claude", Ready: true}} })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a fetch that failed was not tried again")
	}
	if !read()[0].Ready || tries.Load() != 3 {
		t.Errorf("after %d tries the catalog offers %+v", tries.Load(), read())
	}
}
