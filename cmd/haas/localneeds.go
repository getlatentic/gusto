package main

import (
	"os/exec"
	"runtime"
)

// localNeeds is what haas on a person's computer needs that the computer does not have, each said with how to get it. Every
// conversation keeps its files in a git repository, so a computer with no git cannot hold one.
func localNeeds() []string {
	if _, err := exec.LookPath("git"); err == nil {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{"haas needs Git, which keeps each conversation's files: run xcode-select --install"}
	case "windows":
		return []string{"haas needs Git, which keeps each conversation's files: run winget install --id Git.Git -e, or get it from git-scm.com"}
	}
	return []string{"haas needs Git, which keeps each conversation's files: install git with this computer's package manager"}
}
