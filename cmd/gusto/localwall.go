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

// vmLayout is where the virtual machine sandbox is kept for the gusto whose data is at home.
func vmLayout(home string) (vmwall.Layout, error) {
	user, err := os.UserHomeDir()
	if err != nil {
		return vmwall.Layout{}, err
	}
	return vmwall.Layout{Root: filepath.Join(home, "openshell"), User: user}, nil
}

// useInstalledWall has this gusto use the virtual machine sandbox the person installed, where there is one.
func useInstalledWall(home string) {
	if layout, err := vmLayout(home); err == nil {
		vmwall.Adopt(layout, os.Getenv, func(name, value string) { os.Setenv(name, value) })
	}
}

// uninstallSandbox takes away the virtual machine sandbox: the strongest gusto can give a person's tasks.
func uninstallSandbox(home string) int {
	layout, err := vmLayout(home)
	if err != nil {
		return fail(err)
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(layout.User, ".config")
	}
	if err := vmwall.Remove(layout, config); err != nil {
		return fail(err)
	}
	fmt.Println("The virtual machine sandbox is gone. Restart gusto and it chooses another: gusto local install")
	return 0
}

type sandboxInstallFlags struct {
	yes   bool
	image string
}

func declareSandboxInstall(set *flag.FlagSet) *sandboxInstallFlags {
	f := &sandboxInstallFlags{}
	set.BoolVar(&f.yes, "yes", false, "agree to the download without being asked")
	set.StringVar(&f.image, "image", "", "an archive `file` of the root file system for the virtual machines, instead of the one gusto publishes")
	return f
}

// installSandbox downloads and sets up the virtual machine sandbox: the strongest gusto can give a person's tasks.
func installSandbox(home string, args []string) int {
	set := newSet("local sandbox install")
	f := declareSandboxInstall(set)
	if code, ok := parseFlags("local sandbox install", set, args); !ok {
		return code
	}
	if set.NArg() > 0 {
		return usageError("local sandbox install", "takes no arguments")
	}
	layout, err := vmLayout(home)
	if err != nil {
		return fail(err)
	}
	plan, err := vmwall.Prepare(context.Background(), vmwall.This(release, os.Stdout), home, layout.User, f.image)
	if err != nil {
		return fail(err)
	}
	fmt.Println(plan.Words())
	if !f.yes && !agreed(os.Stdin, os.Stdout) {
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
	scanCommand()
	fmt.Println("Done. gusto uses the virtual machine sandbox the next time it starts: gusto local install")
	return 0
}

// agreed asks the person once, and takes only a yes.
func agreed(in *os.File, out *os.File) bool {
	fmt.Fprint(out, "Go ahead? [y/N] ")
	answer, _ := bufio.NewReader(in).ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}
