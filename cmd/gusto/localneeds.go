package main

import (
	"os/exec"
	"runtime"
)

// localNeeds is what gusto on a person's computer needs that the computer does not have, each said with how to get it. Every
// conversation keeps its files in a git repository, so a computer with no git cannot hold one.
func localNeeds() []string {
	if hasGit() {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{"gusto needs Git, which keeps each conversation's files: run xcode-select --install"}
	case "windows":
		return []string{"gusto needs Git, which keeps each conversation's files: run winget install --id Git.Git -e, or get it from git-scm.com"}
	}
	return []string{"gusto needs Git, which keeps each conversation's files: install git with this computer's package manager"}
}

// hasGit says whether git can run. On macOS /usr/bin/git is there before git is: it asks to install the developer tools,
// and xcode-select says whether they are, without asking.
func hasGit() bool {
	found, err := exec.LookPath("git")
	if err != nil {
		return false
	}
	if runtime.GOOS == "darwin" && found == "/usr/bin/git" {
		return exec.Command("/usr/bin/xcode-select", "-p").Run() == nil
	}
	return true
}
