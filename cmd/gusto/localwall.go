package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/internal/vmwall"
)

const localWallUsage = "usage: gusto local wall install [--yes] [--image FILE], or gusto local wall uninstall"

// vmLayout is where the virtual machine wall is kept for the gusto whose data is at home.
func vmLayout(home string) (vmwall.Layout, error) {
	user, err := os.UserHomeDir()
	if err != nil {
		return vmwall.Layout{}, err
	}
	return vmwall.Layout{Root: filepath.Join(home, "openshell"), User: user}, nil
}

// useInstalledWall has this gusto use the virtual machine wall the person installed, where there is one.
func useInstalledWall(home string) {
	if layout, err := vmLayout(home); err == nil {
		vmwall.Adopt(layout, os.Getenv, func(name, value string) { os.Setenv(name, value) })
	}
}

// localWall installs, or takes away, the virtual machine wall: the strongest gusto can give a person's jobs.
func localWall(home string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, localWallUsage)
		return 2
	}
	layout, err := vmLayout(home)
	if err != nil {
		return fail(err)
	}
	switch args[0] {
	case "install":
		return installWall(home, layout, args[1:])
	case "uninstall":
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(layout.User, ".config")
		}
		if err := vmwall.Remove(layout, config); err != nil {
			return fail(err)
		}
		fmt.Println("The virtual machine wall is gone. Restart gusto and it chooses another: gusto local install")
		return 0
	}
	fmt.Fprintln(os.Stderr, localWallUsage)
	return 2
}

func installWall(home string, layout vmwall.Layout, args []string) int {
	flags := flag.NewFlagSet("wall install", flag.ContinueOnError)
	yes := flags.Bool("yes", false, "agree to the download without being asked")
	image := flags.String("image", "", "a root file system archive for the virtual machines, instead of the one gusto publishes")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	plan, err := vmwall.Prepare(context.Background(), vmwall.This(release, os.Stdout), home, layout.User, *image)
	if err != nil {
		return fail(err)
	}
	fmt.Println(plan.Words())
	if !*yes && !agreed(os.Stdin, os.Stdout) {
		fmt.Println("Nothing was downloaded.")
		return 1
	}
	if err := plan.Apply(context.Background()); err != nil {
		return fail(err)
	}
	// The first question to a new image waits for the gateway to import it, which takes about a minute: the person waits for it
	// here, where they are watching, and not when gusto starts.
	fmt.Println("Preparing the agents' image, which takes about a minute the first time...")
	useInstalledWall(home)
	os.Setenv("GUSTO_SANDBOX", "openshell")
	surveyCommand(nil)
	fmt.Println("Done. gusto uses the virtual machine wall the next time it starts: gusto local install")
	return 0
}

// agreed asks the person once, and takes only a yes.
func agreed(in *os.File, out *os.File) bool {
	fmt.Fprint(out, "Go ahead? [y/N] ")
	answer, _ := bufio.NewReader(in).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}
