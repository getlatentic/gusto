package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync/atomic"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/sandbox"
)

// awaitImage is the catalog of a server whose containers could run nothing when it started, while the runner image can
// be fetched. A server kept running across a release starts before its new image is here, and the image arrives only with
// the first container, so every agent would read as not on this server until the server started again. The image is
// fetched in the background and asked again; pages show what the start found until then. read is nil when there is
// nothing to wait for, and done closes when the waiting ends.
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
		if err := fetch(ctx); err != nil {
			fmt.Println("the agents stay not offered:", err)
			return
		}
		fresh := survey()
		current.Store(&fresh)
	}()
	return func() []catalog.Entry { return *current.Load() }, ended
}

// imageFetched says whether this server's containers start from an image it can fetch when it does not have it.
func imageFetched() bool {
	return !runsHere() && os.Getenv("GUSTO_SANDBOX") != "openshell" && os.Getenv(sandbox.ImageSource) != ""
}
