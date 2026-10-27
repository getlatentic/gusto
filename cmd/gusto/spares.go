package main

import (
	"context"
	"os"
	"time"

	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/spare"
	"github.com/getlatentic/gusto/pkg/harness"
)

// spareComputers is the one blank computer this server keeps ready, so the first task of a person who has no computer yet
// does not wait for the gateway to make one. It is on when GUSTO_SPARE_COMPUTER is on.
type spareComputers struct {
	pool *spare.Pool
	key  string
}

// sparePolicy bounds the pool. A computer takes about eight seconds to make, so five minutes is a gateway that is not
// going to answer, and a failure is tried again after ten seconds, then twice as long each time up to ten minutes.
var sparePolicy = spare.Timing{MakeWithin: 5 * time.Minute, RemoveWithin: 2 * time.Minute, RetryFirst: 10 * time.Second, RetryMost: 10 * time.Minute}

// openSpareComputers starts keeping a spare computer for the organization until ctx ends. It returns nothing when the
// setting is off.
func openSpareComputers(ctx context.Context, cfg openshell.Config, org, modelURL string) (*spareComputers, error) {
	if os.Getenv("GUSTO_SPARE_COMPUTER") != "on" {
		return nil, nil
	}
	key, err := openshell.BlankKey(cfg, modelURL)
	if err != nil {
		return nil, err
	}
	maker := spare.Maker{
		Name:   func() string { return computer.SpareName(org) },
		Make:   func(ctx context.Context, name string) error { return openshell.MakeBlank(ctx, cfg, name, modelURL) },
		Remove: func(ctx context.Context, name string) error { return openshell.Remove(ctx, cfg, name) },
		Stale: func(ctx context.Context) ([]string, error) {
			return openshell.Named(ctx, cfg, computer.SparePrefix(org))
		},
	}
	return &spareComputers{pool: spare.Open(ctx, key, maker, sparePolicy), key: key}, nil
}

// close removes what the pool holds before the server ends.
func (s *spareComputers) close() {
	if s != nil {
		s.pool.Close(20 * time.Second)
	}
}

// offerFor is the spare computer a person who has no computer is given, when the computer they would be made is the one
// that was made ahead: the same policy for their agent. It is nil when there is none to offer.
func (s *spareComputers) offerFor(cfg openshell.Config, modelURL string, adapter harness.Adapter) computer.Spares {
	if s == nil {
		return nil
	}
	policy, err := openshell.ComputerPolicy(cfg, modelURL, adapter)
	if err != nil || openshell.PolicyKey(policy) != s.key {
		return nil
	}
	return spare.Offer{Pool: s.pool, Key: s.key}
}
