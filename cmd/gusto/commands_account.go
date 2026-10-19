package main

import (
	"flag"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
)

var envHelperServer = cmdline.Param{Name: "GUSTO_SERVER", Text: "your organisation's server, when -server is not given"}

func accountCommands() []*cmdline.Command {
	return []*cmdline.Command{
		{
			Name: "login", Group: groupAccount, Summary: "Sign this computer in as you, by your organisation's server",
			Usage:       []string{"[-server <address>] [-label <name>] [-browser=false]"},
			Description: "Opens your organisation's approval page in the browser, and keeps the token it gives in this computer's keychain.",
			Flags:       loginFlags,
			Env:         []cmdline.Param{envHelperServer},
			Examples:    []string{"gusto login -server https://acme.gusto.example.com"},
			SeeAlso:     []string{"whoami", "logout"},
			Run:         login,
		},
		{
			Name: "logout", Group: groupAccount, Summary: "End this computer's sign-in and forget it",
			Usage:   []string{"[-server <address>]"},
			Flags:   serverFlag,
			Env:     []cmdline.Param{envHelperServer},
			SeeAlso: []string{"login"},
			Run:     logout,
		},
		{
			Name: "whoami", Group: groupAccount, Summary: "Who this computer is signed in as",
			Usage:   []string{"[-server <address>]"},
			Flags:   serverFlag,
			Env:     []cmdline.Param{envHelperServer},
			SeeAlso: []string{"login"},
			Run:     whoami,
		},
	}
}

func recordCommands() []*cmdline.Command {
	return []*cmdline.Command{
		{
			Name: "keygen", Group: groupRecords, Summary: "Make a key pair to sign records with",
			Usage: []string{"<name> [-dir <dir>] [-awskms <key>]"},
			Description: "Writes a signer's key pair: the private key for GUSTO_SIGNING_KEY on the machine that runs tasks, and the " +
				"public key for every verifier's GUSTO_KEYRING, where its file name is the name verify gives the signer. With -awskms the " +
				"private key stays in AWS KMS and only the public key is written.",
			Args:     []cmdline.Param{{Name: "<name>", Text: "what the signer is called"}},
			Flags:    flagsOf(declareKeygen),
			Env:      []cmdline.Param{envSigning, envKeyring},
			Examples: []string{"gusto keygen ci -dir keys"},
			SeeAlso:  []string{"keys", "verify"},
			Run:      makeKey,
		},
		{
			Name: "keys", Group: groupRecords, Summary: "Which key signed each record",
			Usage: []string{"[-tasks <dir>]"},
			Description: "Says which key stands behind every signed record in the tasks directory, and which records no key in the keyring " +
				"can vouch for. Run it before a public key is removed from a keyring, and after one is restored.",
			Flags: flagsOf(declareKeys),
			Env:   []cmdline.Param{envKeyring, {Name: "GUSTO_JOBS", Text: "the directory of tasks, when -tasks is not given (default jobs)"}},
			Exit: []cmdline.Param{
				{Name: "0", Text: "every record is attributed"},
				{Name: "1", Text: "a record cannot be attributed"},
				{Name: "2", Text: "usage error"},
			},
			SeeAlso: []string{"keygen", "verify"},
			Run:     checkKeys,
		},
		{
			Name: "changes", Group: groupRecords, Summary: "Check a record of platform changes",
			Description: "The platform keeps a signed, chained record of what changes in the settings that decide who may sign in and what they may do.",
			Subs: []*cmdline.Command{{
				Name: "verify", Summary: "Check an exported record and say who signed it",
				Usage: []string{"<file> [-head <hash>]"},
				Description: "Checks that the chain holds and says who signed each event. With -head it also checks that a head hash you kept " +
					"earlier is still in the record, which shows whether lines were cut off the end.",
				Args:     []cmdline.Param{{Name: "<file>", Text: "the exported record"}},
				Flags:    flagsOf(declareChangesVerify),
				Env:      []cmdline.Param{envKeyring},
				Examples: []string{"gusto changes verify changes.jsonl -head 9b1c..."},
				Exit: []cmdline.Param{
					{Name: "0", Text: "the chain holds"},
					{Name: "1", Text: "the chain does not hold, or the head is missing"},
					{Name: "2", Text: "usage error"},
				},
				Run: changesVerify,
			}},
		},
		{
			Name: "retention", Group: groupRecords, Summary: "List what a retention sweep would remove now",
			Usage: []string{"[-tasks <dir>] [-dir <dir>]"},
			Description: "Lists what a sweep would remove now and how much it frees, and removes nothing. The server sweeps on its own " +
				"schedule; this is how an operator sees the effect of a policy before it applies.",
			Flags: flagsOf(declareRetention),
			Env: []cmdline.Param{
				{Name: "GUSTO_RETENTION", Text: "the retention directory, with policy.json and the holds, when -dir is not given"},
				{Name: "GUSTO_JOBS", Text: "the directory of tasks, when -tasks is not given (default jobs)"},
			},
			SeeAlso: []string{"data", "spend"},
			Run:     retentionCommand,
		},
		{
			Name: "data", Group: groupRecords, Summary: "Show, check and roll back the format of the data",
			Description: "Every data directory has a format. These commands show each one's format, say what starting this release would do to it, and take it back to what an earlier release wrote.",
			Subs: []*cmdline.Command{
				{
					Name: "status", Summary: "The format of each data directory",
					Description: "Prints each directory, the format it is at, the release that wrote it and the format this release reads.",
					Run:         none("data status", dataStatus),
				},
				{
					Name: "check", Summary: "What starting this release would do to the data",
					Description: "Says what starting this release would do to the data and whether it could be undone. It changes nothing.",
					Exit: []cmdline.Param{
						{Name: "0", Text: "the server would start"},
						{Name: "1", Text: "the server would refuse to start"},
						{Name: "2", Text: "usage error"},
					},
					Run: none("data check", dataCheck),
				},
				{
					Name: "rollback", Summary: "Take the data back to what an earlier release wrote",
					Usage:       []string{"-to <release>"},
					Description: "Takes every data directory back to the format a named release wrote, so that release can run on it. It checks every directory first and changes none if any cannot go back.",
					Flags:       flagsOf(declareRollback),
					Examples:    []string{"gusto data rollback -to v0.3.0"},
					Run:         dataRollback,
				},
			},
			Env: []cmdline.Param{
				{Name: "GUSTO_JOBS", Text: "tasks (default jobs)"},
				{Name: "GUSTO_SESSIONS", Text: "agents' conversations (default sessions)"},
				{Name: "GUSTO_MIGRATE_IRREVERSIBLE", Text: "1 allows a step that cannot be undone"},
			},
		},
		{
			Name: "spend", Group: groupRecords, Summary: "What the tasks spent this month, by team, project and task",
			Usage: []string{"[-tasks <dir>] [-month <2006-01>] [-json]"},
			Description: "Rebuilds the ledger from the tasks' own proxy records, through the same counting the server's caps use, so it " +
				"needs no server.",
			Flags:    flagsOf(declareSpend),
			Env:      []cmdline.Param{{Name: "GUSTO_JOBS", Text: "the directory of tasks, when -tasks is not given (default jobs)"}},
			Examples: []string{"gusto spend -month 2026-10 -json"},
			SeeAlso:  []string{"retention"},
			Run:      spendReport,
		},
	}
}

func otherCommands() []*cmdline.Command {
	return []*cmdline.Command{
		{
			Name: "agents", Group: groupOther, Summary: "List the agents this binary knows",
			Description: "Lists each agent, the program it runs and what it needs.",
			SeeAlso:     []string{"new", "compare"},
			Run:         none("agents", agentsCommand),
		},
		{
			Name: "version", Group: groupOther, Summary: "Print the release",
			Description: "Prints which release this is. gusto --version does the same.",
			Run:         none("version", versionCommand),
		},
		{
			Name: "help", Group: groupOther, Summary: "Show help for gusto or one command",
			Usage: []string{"[<command> [<subcommand>]]", "--markdown"},
			Description: "With no argument, lists the commands. With a command, shows its arguments, flags, environment, examples and " +
				"exit codes. With --markdown, prints the whole reference, which is what docs/CLI.md holds.",
			Flags:    flagsOf(declareHelp),
			Examples: []string{"gusto help pr open", "gusto local -h"},
			Run:      func(args []string) int { return program().HelpCommand(args) },
		},
		{
			Name: "sandbox-run", Hidden: true, Summary: "Hold a command to the Linux Landlock sandbox",
			Usage: []string{"<policy file> -- <command> [<args>...]", "--check [--network]"},
			Description: "How gusto holds a task to Landlock on Linux: it restricts itself to the policy in the file and becomes the " +
				"command, which keeps the restriction. With --check it applies a policy to a process that does nothing, and says the " +
				"kernel's version of it. Gusto runs this itself.",
			Run: sandboxRun,
		},
	}
}

// The flags of login, logout and whoami are read by the helper they run; these declarations are what help says of them.
func serverFlag(set *flag.FlagSet) {
	set.String("server", "", "the `address` of your organisation's server (default $GUSTO_SERVER)")
}

func loginFlags(set *flag.FlagSet) {
	serverFlag(set)
	set.String("label", "", "what to call this computer in your list of approved ones (default its host name)")
	set.Bool("browser", true, "open the approval page in your browser")
}

func declareHelp(set *flag.FlagSet) *bool {
	return set.Bool("markdown", false, "print the whole reference as Markdown")
}
