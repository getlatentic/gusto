//go:build !windows

package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// loginShellPath is the PATH the person's own shell sets, asked of it as a login shell that reads their settings. An app on
// macOS, and a service, start with a bare PATH, and the engines a person installs are where their shell finds them.
func loginShellPath() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
		if runtime.GOOS == "darwin" {
			shell = "/bin/zsh"
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const mark = "__haas_path__"
	out, _ := exec.CommandContext(ctx, shell, "-ilc", `printf '`+mark+`%s`+mark+`' "$PATH"`).Output()
	_, path, found := strings.Cut(string(out), mark)
	path, _, ended := strings.Cut(path, mark)
	if !found || !ended {
		return ""
	}
	return path
}
