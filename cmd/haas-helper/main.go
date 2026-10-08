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
		"login":  cli.Login,
		"logout": cli.Logout,
		"whoami": cli.Whoami,
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
`)
}
