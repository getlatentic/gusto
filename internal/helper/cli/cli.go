// Package cli is the commands of the helper on a person's computer. They are here and not in cmd/gusto so that a program with
// only these commands builds on every operating system, which the server's own program does not.
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"

	"github.com/getlatentic/gusto/internal/helper"
)

// serverFlag reads the address of the server a command talks to: the flag, or GUSTO_SERVER.
func serverFlag(flags *flag.FlagSet) *string {
	return flags.String("server", os.Getenv("GUSTO_SERVER"), "the address of your organisation's server (default GUSTO_SERVER)")
}

func needServer(address string) (string, bool) {
	if address == "" {
		fmt.Fprintln(os.Stderr, "name the server: -server https://gusto.example.com, or set GUSTO_SERVER")
		return "", false
	}
	return helper.Server(address), true
}

// Login signs this computer in as its person, by the device flow, and keeps the token in the keychain.
func Login(args []string) int {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	address := serverFlag(flags)
	hostname, _ := os.Hostname()
	label := flags.String("label", hostname, "what to call this computer in your list of approved ones")
	open := flags.Bool("browser", true, "open the approval page in your browser")
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	token, person, err := helper.Login{
		Server: server, Label: *label,
		Show: func(address, code string) {
			fmt.Printf("To sign this computer in, open\n\n  %s\n\nand check that the code is %s.\n\nWaiting for you to approve it…\n", address, code)
			if *open {
				openBrowser(address)
			}
		},
	}.Run(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := vault.Set(server, token); err != nil {
		fmt.Fprintln(os.Stderr, "the token could not be kept:", err)
		return 1
	}
	fmt.Printf("Signed in to %s as %s.\n", server, person)
	return 0
}

// Whoami says who this computer is signed in as, asking the server so a token that was ended is found out.
func Whoami(args []string) int {
	flags := flag.NewFlagSet("whoami", flag.ContinueOnError)
	address := serverFlag(flags)
	if flags.Parse(args) != nil {
		return 2
	}
	server, ok := needServer(*address)
	if !ok {
		return 2
	}
	client, code := signedIn(server)
	if code != 0 {
		return code
	}
	who, err := client.Me(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("%s on %s (%s)\n", who.Person, server, who.Role)
	return 0
}

// Logout ends this computer's token on the server, then forgets it. When the server cannot be reached the token is
// forgotten anyway and the person is told it can still be ended on the server's own page.
func Logout(args []string) int {
	flags := flag.NewFlagSet("logout", flag.ContinueOnError)
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
	token, err := vault.Get(server)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := (helper.Client{Server: server, Token: token}).End(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "the server could not end the token (%v): end it at %s/device\n", err, server)
	}
	if err := vault.Delete(server); err != nil {
		fmt.Fprintln(os.Stderr, "the token could not be removed from this computer:", err)
		return 1
	}
	fmt.Println("Signed out of", server)
	return 0
}

// signedIn is a client with the token kept for server, or the exit code of saying there is none.
func signedIn(server string) (helper.Client, int) {
	vault, err := helper.OpenVault(os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return helper.Client{}, 1
	}
	token, err := vault.Get(server)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return helper.Client{}, 1
	}
	return helper.Client{Server: server, Token: token}, 0
}

// openBrowser asks the operating system to open an address, and says nothing when it cannot: the address is printed.
func openBrowser(address string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", address)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	_ = command.Start()
}

// ShowVersion says which release the helper is.
func ShowVersion(args []string) int {
	fmt.Println("gusto-helper", Version)
	return 0
}
