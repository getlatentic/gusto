package main

import "github.com/getlatentic/gusto/cmd/internal/cmdline"

// boxCommands are what gusto runs inside the boxes it starts, and nobody runs by hand.
func boxCommands() []*cmdline.Command {
	return []*cmdline.Command{
		{
			Name: "sandbox-run", Hidden: true, Summary: "Hold a command to the Linux Landlock sandbox",
			Usage: []string{"<policy file> -- <command> [<args>...]", "--check [--network]"},
			Description: "How gusto holds a task to Landlock on Linux: it restricts itself to the policy in the file and becomes the " +
				"command, which keeps the restriction. With --check it applies a policy to a process that does nothing, and says the " +
				"kernel's version of it. Gusto runs this itself.",
			Run: sandboxRun,
		},
		{
			Name: "boxed", Hidden: true, Summary: "Start an agent in a sandbox with its run's own way to its model",
			Usage: []string{"--model <proxy> [--policy <json>] -- <command> [<args>...]"},
			Description: "What a sandbox runs in place of an agent: it starts the run's forwarder, which trades the run's secret for a " +
				"token bound to a key of its own, points the agent at it, holds itself to the Landlock policy when one is given, and " +
				"becomes the agent. Gusto runs this itself.",
			Env: cmdline.ParamsOf(new(boxedSettings).declare), Run: boxed,
		},
		{
			Name: "boxed-forward", Hidden: true, Summary: "The forwarder gusto boxed starts",
			Usage:       []string{"<proxy>"},
			Description: "Passes the agent's calls to the proxy with the run's token and a proof made for each. Gusto runs this itself.",
			Run:         boxedForward,
		},
		{
			Name: "boxed-sign", Hidden: true, Summary: "The signer the forwarder starts",
			Description: "Holds the run's key, where no other process can read it, and makes the forwarder's proofs. Gusto runs this itself.",
			Run:         boxedSign,
		},
		{
			Name: "shared-servers", Hidden: true, Summary: "Run many organizations' servers in one process, for gusto cloud",
			Usage: []string{"--socket <path>"},
			Description: "What gusto cloud starts when GUSTO_CLOUD_SERVERS is shared: one process that runs each organization's " +
				"server as gusto cloud asks on the socket, each with its own settings, ports and data. Gusto runs this itself.",
			Run: sharedServers,
		},
		{
			Name: "inside-app", Hidden: true, Summary: "Run an app an agent built, in a box of its own",
			Usage: []string{"--socket <path> --port <port> --dir <folder> -- <command>"},
			Description: "What an app's container runs: it starts the command in the folder as the agent, with PORT set, and passes " +
				"each connection to the socket on to that port. Gusto runs this itself.",
			Env: cmdline.ParamsOf(new(insideSettings).declare), Run: insideApp,
		},
	}
}
