package main

import (
	"flag"
	"time"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
	"github.com/getlatentic/gusto/internal/helper/cli"
)

const (
	groupSignIn = "Sign in"
	groupWork   = "Work"
	groupOther  = "Other"
)

var envServer = cmdline.Param{Name: "GUSTO_SERVER", Text: "your organisation's server, when -server is not given"}

// program is the command table. The flags of each command are read by the command itself, in internal/helper/cli; the
// declarations here are what help says of them.
func program() *cmdline.Program {
	return &cmdline.Program{
		Name:    "gusto-helper",
		Tagline: "gusto-helper is your computer's way to your organisation's agents.",
		Groups:  []string{groupSignIn, groupWork, groupOther},
		Exit: []cmdline.Param{
			{Name: "0", Text: "success"},
			{Name: "1", Text: "failure"},
			{Name: "2", Text: "usage error"},
		},
		Commands: []*cmdline.Command{
			{
				Name: "login", Group: groupSignIn, Summary: "Sign this computer in as you",
				Usage:       []string{"[-server <address>] [-label <name>] [-browser=false]"},
				Description: "Opens your organisation's approval page in the browser, and keeps the token it gives in this computer's keychain.",
				Flags:       loginFlags,
				Env:         []cmdline.Param{envServer},
				Examples:    []string{"gusto-helper login -server https://acme.gusto.example.com"},
				SeeAlso:     []string{"whoami", "logout"},
				Run:         cli.Login,
			},
			{
				Name: "logout", Group: groupSignIn, Summary: "End this computer's sign-in and forget it",
				Usage: []string{"[-server <address>]"}, Flags: serverFlag, Env: []cmdline.Param{envServer},
				SeeAlso: []string{"login"},
				Run:     cli.Logout,
			},
			{
				Name: "whoami", Group: groupSignIn, Summary: "Who this computer is signed in as",
				Usage: []string{"[-server <address>]"}, Flags: serverFlag, Env: []cmdline.Param{envServer},
				SeeAlso: []string{"login"},
				Run:     cli.Whoami,
			},
			{
				Name: "run", Group: groupWork, Summary: "Follow your tasks and answer what this computer is asked",
				Usage:       []string{"[-server <address>] [-tray=false]"},
				Description: "Runs the helper in the foreground. A service starts this at login; see gusto-helper service.",
				Flags:       runFlags,
				Env:         []cmdline.Param{envServer},
				SeeAlso:     []string{"service", "status"},
				Run:         cli.Run,
			},
			{
				Name: "status", Group: groupWork, Summary: "What the running helper says of itself",
				SeeAlso: []string{"run"},
				Run:     cli.Status,
			},
			{
				Name: "service", Group: groupWork, Summary: "Start the helper at login",
				Description: "Installs or removes the helper's start at login, or says whether it is installed.",
				SeeAlso:     []string{"run"},
				Subs: []*cmdline.Command{
					serviceCommand("install", "Start the helper at login"),
					serviceCommand("uninstall", "Stop starting the helper at login"),
					serviceCommand("status", "Say whether the helper starts at login"),
				},
			},
			{
				Name: "sync", Group: groupWork, Summary: "Keep a folder here the same as your agent's cloud folder",
				Usage: []string{"-folder <dir> [-server <address>] [-instance <name>] [flags]"},
				Description: "Compares the folder here with your agent's folder in the cloud every few seconds. A change on one side goes " +
					"to the other, a change beats a deletion, and when both sides changed a file the local one keeps its name and the " +
					"cloud's is saved beside it.",
				Flags:    syncFlags,
				Env:      []cmdline.Param{envServer, {Name: "GUSTO_INSTANCE", Text: "the agent whose folder this is, when -instance is not given"}},
				Examples: []string{"gusto-helper sync -folder ~/agent-files"},
				SeeAlso:  []string{"acp"},
				Run:      cli.Sync,
			},
			{
				Name: "acp", Group: groupWork, Summary: "Be an agent for editors that speak the Agent Client Protocol",
				Usage:       []string{"[-server <address>] [-instance <name>] [-agent <kind>]"},
				Description: "An editor starts this and talks to it on standard input and output. Whatever the helper has to say goes to standard error.",
				Flags:       acpFlags,
				Env: []cmdline.Param{envServer,
					{Name: "GUSTO_INSTANCE", Text: "the agent to work with, when -instance is not given"},
					{Name: "GUSTO_AGENT_KIND", Text: "the kind of agent to start, when -agent is not given"}},
				SeeAlso: []string{"sync"},
				Run:     cli.ACP,
			},
			{
				Name: "version", Group: groupOther, Summary: "Print the release",
				Description: "Prints which release this is. gusto-helper --version does the same.",
				Run:         cli.ShowVersion,
			},
			{
				Name: "help", Group: groupOther, Summary: "Show help for gusto-helper or one command",
				Usage:       []string{"[<command> [<subcommand>]]", "--markdown"},
				Description: "With no argument, lists the commands. With a command, shows its arguments, flags, environment and examples.",
				Flags:       func(set *flag.FlagSet) { set.Bool("markdown", false, "print the whole reference as Markdown") },
				Run:         func(args []string) int { return program().HelpCommand(args) },
			},
		},
	}
}

func serviceCommand(name, summary string) *cmdline.Command {
	return &cmdline.Command{
		Name: name, Summary: summary,
		Usage: []string{"[-server <address>]"}, Flags: serverFlag, Env: []cmdline.Param{envServer},
		Run: func(args []string) int { return cli.Service(append([]string{name}, args...)) },
	}
}

func serverFlag(set *flag.FlagSet) {
	set.String("server", "", "the `address` of your organisation's server (default $GUSTO_SERVER)")
}

func loginFlags(set *flag.FlagSet) {
	serverFlag(set)
	set.String("label", "", "what to call this computer in your list of approved ones (default its host name)")
	set.Bool("browser", true, "open the approval page in your browser")
}

func runFlags(set *flag.FlagSet) {
	serverFlag(set)
	set.Bool("tray", true, "show the icon in the menu bar or tray, and a notification when a turn ends")
}

func syncFlags(set *flag.FlagSet) {
	serverFlag(set)
	set.String("instance", "", "the `name` of the agent in the cloud whose folder this is; your newest by default")
	set.String("folder", "", "the `dir` on this computer to keep the same")
	set.Bool("once", false, "compare once and stop")
	set.Bool("dry-run", false, "say what would be done, and do nothing")
	set.Bool("force", false, "go on when the checks say it looks like a mistake")
	set.Duration("every", 3*time.Second, "how often to compare while there are changes; it waits longer when there are none")
}

func acpFlags(set *flag.FlagSet) {
	serverFlag(set)
	set.String("instance", "", "the `name` of the agent in the cloud to work with; your newest, or a new one, by default")
	set.String("agent", "", "the `kind` of agent to start when you have none; the first this server can run by default")
}
