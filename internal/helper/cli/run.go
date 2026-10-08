package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/getlatentic/haas/internal/helper"
	"github.com/getlatentic/haas/internal/helper/service"
)

// settingsDir is where the helper keeps what it writes: beside the token file, in the person's own settings folder.
func settingsDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "haas"), nil
}

// Run is the helper itself, in the foreground. A service starts this.
func Run(args []string) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	address := serverFlag(flags)
	if flags.Parse(args) != nil {
		return 2
	}
	server, ok := needServer(*address)
	if !ok {
		return 2
	}
	vault, err := helper.OpenVault(os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	dir, err := settingsDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Println("following", server)
	if err := (&helper.Daemon{Server: server, Vault: vault, Dir: dir}).Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// Status says what the running helper says of itself.
func Status(args []string) int {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	if flags.Parse(args) != nil {
		return 2
	}
	dir, err := settingsDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	local, err := helper.Discover(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	status, err := local.Ask(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "the helper does not answer:", err)
		return 1
	}
	switch {
	case !status.SignedIn:
		fmt.Printf("running, following %s, but not signed in: run `haas-helper login`\n", status.Server)
	case !status.Connected:
		fmt.Printf("running, signed in as %s, and trying to reach %s\n", status.Person, status.Server)
	default:
		fmt.Printf("running, signed in as %s, and following %s\n", status.Person, status.Server)
	}
	return 0
}

// Service installs or removes the helper's start at login.
func Service(args []string) int {
	if len(args) == 0 || (args[0] != "install" && args[0] != "uninstall" && args[0] != "status") {
		fmt.Fprintln(os.Stderr, "usage: haas-helper service install|uninstall|status [-server <address>]")
		return 2
	}
	flags := flag.NewFlagSet("service", flag.ContinueOnError)
	address := serverFlag(flags)
	if flags.Parse(args[1:]) != nil {
		return 2
	}
	program, err := os.Executable()
	if err == nil {
		program, err = filepath.EvalSymlinks(program)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	spec := service.Spec{Program: program, Server: helper.Server(*address)}
	manager := service.Here()
	switch args[0] {
	case "install":
		if _, ok := needServer(*address); !ok {
			return 2
		}
		said, err := manager.Install(spec)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("The helper is", said)
	case "uninstall":
		if err := manager.Uninstall(spec); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("The helper no longer starts at login.")
	default:
		if installed, _ := manager.Installed(spec); installed {
			fmt.Println("The helper starts at login.")
		} else {
			fmt.Println("The helper does not start at login. `haas-helper service install` makes it.")
			return 1
		}
	}
	return 0
}
