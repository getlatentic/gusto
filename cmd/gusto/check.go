package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/getlatentic/gusto/internal/review"
)

type verifyPullRequestFlags struct{ into string }

func declareVerifyPullRequest(set *flag.FlagSet) *verifyPullRequestFlags {
	f := &verifyPullRequestFlags{}
	set.StringVar(&f.into, "into", "", "unpack the task in this `directory` and keep it, rather than in one removed afterwards")
	return f
}

// verifyPullRequest verifies a pull request's record from the repository alone: the
// command `gusto pr open` prints in every pull request it opens. It exits the way
// verify does: 0 when everything agrees, and 1 naming what does not.
func verifyPullRequest(args []string) int {
	set := newSet("pr verify")
	f := declareVerifyPullRequest(set)
	if len(args) < 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return usageError("pr verify", "needs the repository and the manifest's sha256, then its flags")
	}
	if code, ok := parseFlags("pr verify", set, args[2:]); !ok {
		return code
	}
	if set.NArg() > 0 {
		return usageError("pr verify", "needs only the repository and the manifest's sha256")
	}
	dir, done, err := unpackInto(f.into)
	if err != nil {
		return fail(err)
	}
	defer done()
	report, err := review.Check(context.Background(), args[0], args[1], dir)
	if err != nil {
		return fail(err)
	}
	for _, line := range report.Lines() {
		fmt.Println(line)
	}
	if !report.Holds() {
		return 1
	}
	return 0
}

// unpackInto is the directory asked for, which is kept, or a new one that is
// removed when the check is done.
func unpackInto(asked string) (string, func(), error) {
	if asked != "" {
		return asked, func() {}, nil
	}
	dir, err := os.MkdirTemp("", "gusto-check-")
	if err != nil {
		return "", nil, err
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}
