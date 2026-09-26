package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/ship"
)

// shipJob opens a pull request from a job whose gate passed. It exits 1 when
// the job must not ship, the way verify exits 1 when the evidence changed, and
// 2 when something went wrong on the way.
func shipJob(args []string) int {
	flags := flag.NewFlagSet("ship", flag.ContinueOnError)
	dry := flags.Bool("dry-run", false, "print what would happen, and do none of it")
	remote := flags.String("remote", "", "where to push, the job's repository by default")
	github := flags.String("github", "", "owner/name on GitHub, read off the remote by default")
	base := flags.String("base", "", "the branch the pull request targets")
	where, rest := split(args)
	if err := flags.Parse(rest); err != nil {
		return 2
	}
	if where == "" || flags.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "haas ship needs a job directory, then its flags")
		return 2
	}
	plan, err := ship.Prepare(job.Dir(where), ship.Options{Remote: *remote, GitHub: *github, Base: *base})
	if err != nil {
		return refusedOr(err)
	}
	shipper, err := shipperFor(*dry)
	if err != nil {
		return fail(err)
	}
	address, err := shipper.Ship(context.Background(), plan)
	if err != nil {
		return fail(err)
	}
	if address != "" {
		fmt.Println(address)
	}
	return 0
}

// shipperFor needs a token only when something will be pushed, so a dry run
// works on a machine that holds none.
func shipperFor(dry bool) (ship.Shipper, error) {
	if dry {
		return ship.Shipper{Out: os.Stdout, DryRun: true}, nil
	}
	forge, err := ship.GitHubFromEnv()
	if err != nil {
		return ship.Shipper{}, err
	}
	return ship.Shipper{Pusher: ship.Git{Token: forge.Token}, Forge: forge, Out: os.Stdout}, nil
}

func refusedOr(err error) int {
	if errors.Is(err, ship.ErrRefused) {
		fmt.Fprintf(os.Stderr, "haas: %v\n", err)
		return 1
	}
	return fail(err)
}
