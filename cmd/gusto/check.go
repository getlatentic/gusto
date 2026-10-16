package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/getlatentic/gusto/internal/review"
)

// checkPull verifies a pull request's evidence from the repository alone: the
// command `gusto ship` prints in every pull request it opens. It exits the way
// verify does: 0 when everything agrees, 1 naming what does not, and 2 when
// the check could not be made.
func checkPull(args []string) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	into := flags.String("into", "", "unpack the job here and keep it, rather than in a directory removed afterwards")
	if len(args) < 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		fmt.Fprintln(os.Stderr, "gusto check needs the repository and the manifest's sha256, then its flags")
		return 2
	}
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() > 0 {
		return 2
	}
	dir, done, err := unpackInto(*into)
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
