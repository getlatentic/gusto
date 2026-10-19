package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/repoaccess"
	"github.com/getlatentic/gusto/internal/ship"
	"github.com/getlatentic/gusto/internal/signing"
)

type openFlags struct {
	dry                  bool
	remote, github, base string
}

func declareOpen(set *flag.FlagSet) *openFlags {
	f := &openFlags{}
	set.BoolVar(&f.dry, "dry-run", false, "print what would happen, and do none of it")
	set.StringVar(&f.remote, "remote", "", "where to push, as a `url`; the task's repository by default")
	set.StringVar(&f.github, "github", "", "`owner/name` on GitHub, read off the remote by default")
	set.StringVar(&f.base, "base", "", "the `branch` the pull request targets")
	return f
}

// openPullRequest opens a pull request from a task whose check passed. It exits 1 when
// the task must not be sent, the way verify exits 1 when the record changed, and
// when something went wrong on the way.
func openPullRequest(args []string) int {
	set := newSet("pr open")
	f := declareOpen(set)
	where, rest := split(args)
	if code, ok := parseFlags("pr open", set, rest); !ok {
		return code
	}
	if where == "" || set.NArg() > 0 {
		return usageError("pr open", "needs one task directory, then its flags")
	}
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return fail(err)
	}
	options := ship.Options{Remote: f.remote, GitHub: f.github, Base: f.base, Keyring: keyring}
	plan, err := ship.Prepare(job.Dir(where), options)
	if err != nil {
		return refusedOr(err)
	}
	shipper, release, err := shipperFor(context.Background(), f.dry, plan)
	if err != nil {
		return fail(err)
	}
	defer release()
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
// works on a machine that holds none. With a GitHub App the token is minted for
// the one repository being pushed to, and revoked when shipping is done; without
// one it is the standing token in GUSTO_GITHUB_TOKEN. The function it returns
// ends the token.
func shipperFor(ctx context.Context, dry bool, plan ship.Plan) (ship.Shipper, func(), error) {
	nothing := func() {}
	if dry {
		return ship.Shipper{Out: os.Stdout, DryRun: true}, nothing, nil
	}
	forge, err := repoaccess.FromEnv()
	if err != nil {
		return ship.Shipper{}, nothing, err
	}
	if repo, ours := forge.Locate(plan.Remote); forge.App != nil && ours {
		token, err := forge.App.Token(ctx, repo.Owner, repo.Name, repoaccess.Write)
		if err != nil {
			return ship.Shipper{}, nothing, err
		}
		hub := ship.GitHub{API: forge.API, Token: token.Value, Client: &http.Client{Timeout: 30 * time.Second}}
		return ship.Shipper{Pusher: ship.Git{Token: token.Value, Host: forge.Web}, Forge: hub, Out: os.Stdout}, func() { repoaccess.Release(token) }, nil
	}
	hub, err := ship.GitHubFromEnv()
	if err != nil {
		return ship.Shipper{}, nothing, err
	}
	return ship.Shipper{Pusher: ship.Git{Token: hub.Token}, Forge: hub, Out: os.Stdout}, nothing, nil
}

func refusedOr(err error) int {
	if errors.Is(err, ship.ErrRefused) {
		fmt.Fprintf(os.Stderr, "gusto: %v\n", err)
		return 1
	}
	return fail(err)
}
