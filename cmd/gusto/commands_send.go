package main

import "github.com/getlatentic/gusto/cmd/internal/cmdline"

func sendCommands() []*cmdline.Command {
	return []*cmdline.Command{
		{
			Name: "pr", Group: groupSend, Summary: "Open a pull request from a task, or verify one",
			Description: "A task whose check passed can become a pull request carrying its record, and anybody can check that record " +
				"from the repository alone.",
			SeeAlso: []string{"verify", "export"},
			Subs:    prCommands(),
		},
		{
			Name: "verify", Group: groupSend, Summary: "Check a task's record: hashes, signer and check",
			Usage: []string{"<dir> [-signed]"},
			Description: "Hashes the task's files again and compares them with the record, names the signer from the keyring, and runs " +
				"the check again. The exit code is 0 when everything agrees and 1 when anything does not.",
			Args:     []cmdline.Param{{Name: "<dir>", Text: "the task directory"}},
			Flags:    flagsOf(declareVerify),
			Env:      []cmdline.Param{envKeyring},
			Examples: []string{"gusto verify fix-login -signed"},
			Exit: []cmdline.Param{
				{Name: "0", Text: "the files match, and the check still gives its recorded answer"},
				{Name: "1", Text: "a file changed, the signature fails, or the check now answers differently"},
				{Name: "2", Text: "usage error"},
			},
			SeeAlso: []string{"pr verify", "export", "keys"},
			Run:     verify,
		},
		{
			Name: "export", Group: groupSend, Summary: "Write an audit record anyone can verify",
			Usage: []string{"<dir> [-to <dir>] [-policy <file>] [-requests <file>]", "-verify <export dir>"},
			Description: "Writes a task's record as an audit export in a new directory, dropping and redacting what the policy says. " +
				"With -verify it checks an export instead of writing one.",
			Args:  []cmdline.Param{{Name: "<dir>", Text: "the task directory, or with -verify the export directory"}},
			Flags: flagsOf(declareExport),
			Env:   []cmdline.Param{envKeyring},
			Examples: []string{
				"gusto export fix-login",
				"gusto export -verify fix-login.audit",
			},
			SeeAlso: []string{"verify"},
			Run:     exportTask,
		},
	}
}

func prCommands() []*cmdline.Command {
	return []*cmdline.Command{
		{
			Name: "open", Summary: "Push a passed task's patch and open a pull request",
			Usage: []string{"<dir> [-dry-run] [-remote <url>] [-github <owner/name>] [-base <branch>]"},
			Description: "Pushes the patch of a task whose check passed and opens a pull request that carries the task's record and the " +
				"command that verifies it. A dry run prints what would happen and does none of it, and needs no token.",
			Args:  []cmdline.Param{{Name: "<dir>", Text: "the task directory"}},
			Flags: flagsOf(declareOpen),
			Env: []cmdline.Param{
				{Name: "GUSTO_GITHUB_TOKEN", Text: "the standing token to push and open with, when there is no GitHub App"},
				envKeyring,
			},
			Examples: []string{"gusto pr open fix-login -dry-run", "gusto pr open fix-login"},
			Exit: []cmdline.Param{
				{Name: "0", Text: "the pull request was opened, or printed by a dry run"},
				{Name: "1", Text: "the task must not be sent, or something went wrong"},
				{Name: "2", Text: "usage error"},
			},
			SeeAlso: []string{"pr verify", "verify"},
			Run:     openPullRequest,
		},
		{
			Name: "verify", Summary: "Fetch a pull request's record and verify it",
			Usage: []string{"<repo> <sha256> [-into <dir>]"},
			Description: "Verifies a pull request's record from the repository alone. This is the command gusto pr open prints in " +
				"every pull request it opens.",
			Args: []cmdline.Param{
				{Name: "<repo>", Text: "the repository the pull request is in"},
				{Name: "<sha256>", Text: "the hash of the record's manifest, as the pull request gives it"},
			},
			Flags:    flagsOf(declareVerifyPullRequest),
			Examples: []string{"gusto pr verify https://github.com/acme/app 3f2a9c..."},
			Exit: []cmdline.Param{
				{Name: "0", Text: "everything agrees"},
				{Name: "1", Text: "something does not, and the output names it, or the check could not be made"},
				{Name: "2", Text: "usage error"},
			},
			SeeAlso: []string{"pr open", "verify"},
			Run:     verifyPullRequest,
		},
	}
}
