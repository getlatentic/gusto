package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/getlatentic/haas/internal/landlock"
)

const sandboxRunUsage = "usage: haas sandbox-run FILE -- command [args...], or haas sandbox-run --check [--network]"

// sandboxRun is how haas holds a job to Landlock on Linux: it restricts itself to the policy in FILE, and becomes the command, which
// keeps the restriction. With --check it applies a policy of its own to a process that does nothing, which is how haas learns the
// sandbox works here, and says the kernel's version of it.
func sandboxRun(args []string) int {
	if len(args) >= 1 && args[0] == "--check" {
		return checkLandlock(len(args) >= 2 && args[1] == "--network")
	}
	if len(args) < 3 || args[1] != "--" {
		fmt.Fprintln(os.Stderr, sandboxRunUsage)
		return 2
	}
	policy, err := landlock.Load(args[0])
	if err != nil {
		return fail(err)
	}
	program, err := exec.LookPath(args[2])
	if err != nil {
		return fail(err)
	}
	if err := landlock.Apply(policy); err != nil {
		return fail(fmt.Errorf("the sandbox did not start, and the job is not run unprotected: %w", err))
	}
	return fail(syscall.Exec(program, args[2:], os.Environ()))
}

// checkLandlock tries the sandbox on this process and prints the kernel's version of it. With network it asks for the network rules too.
func checkLandlock(network bool) int {
	abi, err := landlock.ABI()
	if err != nil {
		return fail(err)
	}
	policy := landlock.Policy{}
	if network {
		policy.ProxyPort = 1
	}
	if err := landlock.Apply(policy); err != nil {
		return fail(err)
	}
	fmt.Println(abi)
	return 0
}
