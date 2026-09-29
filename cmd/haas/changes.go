package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/signing"
)

// openChanges opens the record of what changes on the platform, signed by the
// deployment's key when it holds one. A log that does not hold together stops
// the server starting: what it would go on to write would look as sound as what
// came before it.
func openChanges(keyring signing.Keyring) (*changelog.Log, error) {
	signer, _, err := signing.Configured()
	if err != nil {
		return nil, err
	}
	return changelog.Open(setting("HAAS_CHANGES", filepath.Join("changes", "changes.jsonl")), signer, keyring)
}

// governing is the settings that decide who may sign in and what they may do,
// with nothing secret in them. A change to any is written down the next time the
// server starts.
func governing() map[string]string {
	settings := map[string]string{}
	for _, name := range []string{
		"HAAS_ALLOWED", "HAAS_ADMINS", "HAAS_RUNNER_GROUPS", "HAAS_ADMIN_GROUPS",
		"HAAS_OIDC_GROUPS_CLAIM", "HAAS_OIDC_ISSUER", "HAAS_ALLOWED_HOSTS", "HAAS_MODELS", "HAAS_JOB_NETWORK", "HAAS_API_TOKENS", "HAAS_LIMIT_PERSON", "HAAS_LIMIT_TEAM",
	} {
		settings[name] = os.Getenv(name)
	}
	if path := os.Getenv("HAAS_SPEND_POLICY"); path != "" {
		if text, err := os.ReadFile(path); err == nil {
			sum := sha256.Sum256(text)
			settings["HAAS_SPEND_POLICY sha256"] = hex.EncodeToString(sum[:])
		}
	}
	return settings
}

// changesCommand is `haas changes verify`: it checks an exported record and says
// who signed it, and that lines were not cut off the end.
func changesCommand(args []string) int {
	if len(args) == 0 || args[0] != "verify" {
		usage()
		return 2
	}
	flags := flag.NewFlagSet("changes verify", flag.ContinueOnError)
	head := flags.String("head", "", "a head hash kept earlier, which the record must still contain")
	// The file may come before the flags or after them.
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() == 0 {
		usage()
		return 2
	}
	where := flags.Arg(0)
	if err := flags.Parse(flags.Args()[1:]); err != nil || flags.NArg() > 0 {
		usage()
		return 2
	}
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return fail(err)
	}
	file, err := os.Open(where)
	if err != nil {
		return fail(err)
	}
	report, err := changelog.Verify(file, keyring)
	file.Close()
	if err != nil {
		fmt.Printf("%s\n", err)
		return 1
	}
	fmt.Printf("%d events, and the chain holds\nhead %s\n", report.Events, report.Head)
	names := make([]string, 0, len(report.Signers))
	for name := range report.Signers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("%5d signed by %s\n", report.Signers[name], name)
	}
	for id, count := range report.Unknown {
		fmt.Printf("%5d signed by %s, which is not in the keyring\n", count, id)
	}
	if report.Unsigned > 0 {
		fmt.Printf("%5d not signed\n", report.Unsigned)
	}
	if *head != "" {
		again, err := os.Open(where)
		if err != nil {
			return fail(err)
		}
		defer again.Close()
		seq, found, err := changelog.Contains(again, strings.TrimSpace(*head))
		if err != nil {
			return fail(err)
		}
		if !found {
			fmt.Printf("the head %s is not in this record: lines were cut off the end, or it is another record\n", *head)
			return 1
		}
		fmt.Printf("the head you kept is line %d, and it is still here\n", seq)
	}
	return 0
}
