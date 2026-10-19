// Command gusto-helper is what runs on a person's own computer to reach their organization's agents: it signs in once, keeps
// its token in the operating system's keychain, and serves the local work. It is a small program of its own, not the server's,
// so that it builds and runs on macOS, Windows and Linux alike.
package main

import "os"

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int { return program().Run(args) }
