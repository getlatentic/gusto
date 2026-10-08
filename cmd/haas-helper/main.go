// Command haas-helper is what runs on a person's own computer to reach their organisation's agents: it signs in once, keeps
// its token in the operating system's keychain, and serves the local work. It is a small program of its own, not the server's,
// so that it builds and runs on macOS, Windows and Linux alike.
package main

import (
	"fmt"
	"os"

	"github.com/getlatentic/haas/internal/helper/cli"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	verb, rest := args[0], args[1:]
	do, found := map[string]func([]string) int{
		"login":   cli.Login,
		"logout":  cli.Logout,
		"whoami":  cli.Whoami,
		"run":     cli.Run,
		"status":  cli.Status,
		"service": cli.Service,
		"acp":     cli.ACP,
		"sync":    cli.Sync,
		"version": cli.ShowVersion,
	}[verb]
	if !found {
		usage()
		return 2
	}
	return do(rest)
}

func usage() {
	fmt.Fprint(os.Stderr, `haas-helper: your computer's way to your organisation's agents

  haas-helper login   sign this computer in as you [-server, -label]
  haas-helper logout  end this computer's sign-in and forget it
  haas-helper whoami  who this computer is signed in as
  haas-helper run     follow your jobs and answer what on this computer asks [-server]
  haas-helper status  what the running helper says of itself
  haas-helper service install|uninstall|status  start the helper at login [-server]
  haas-helper sync    keep a folder here the same as your agent's folder in the cloud [-folder, -instance, -dry-run, -once, -force]
  haas-helper version which release this is
  haas-helper acp     be an agent for an editor that speaks the Agent Client Protocol [-server, -instance, -agent]
`)
}
