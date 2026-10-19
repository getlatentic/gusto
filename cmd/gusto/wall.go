package main

import (
	"fmt"
	"os"

	"github.com/getlatentic/gusto/internal/isolation"
	"github.com/getlatentic/gusto/internal/web"
)

// wall is the choice gusto on a person's own computer made when it started: which sandbox agents run behind. It is nil for a server
// that is not on a person's own computer, whose operator chooses and sees it with gusto doctor.
var wall *isolation.Choice

// runsHere says tasks run in this process, behind no sandbox or behind this computer's own sandbox, and not in a container or a VM.
func runsHere() bool {
	sandbox := os.Getenv("GUSTO_SANDBOX")
	return sandbox == "here" || sandbox == "process"
}

// confirmedFile is where a person's word that they know there is no sandbox is kept, in gusto's own folder.
const confirmedFile = "no-sandbox-confirmed"

// chooseWall is the strongest sandbox this computer can make, set as GUSTO_SANDBOX so the rest of gusto runs tasks behind it. A person or
// an operator who set GUSTO_SANDBOX is not second-guessed.
func chooseWall() isolation.Choice {
	choice := isolation.Choose(isolation.This())
	if !choice.Forced {
		os.Setenv("GUSTO_SANDBOX", map[isolation.Kind]string{isolation.VM: "openshell", isolation.Container: "container", isolation.Process: "process", isolation.Nothing: "here"}[choice.Kind])
		// The runtime that answered is the one tasks are started with, which is podman on a computer that has no docker.
		if choice.Kind == isolation.Container && os.Getenv("GUSTO_CONTAINER") == "" {
			os.Setenv("GUSTO_CONTAINER", choice.Detail)
		}
	}
	wall = &choice
	fmt.Println(choice.Words())
	for _, passed := range choice.Passed {
		fmt.Printf("  no %s: %s\n", passed.Kind, passed.Detail)
		if passed.Cause != "" {
			fmt.Printf("      %s\n", passed.Cause)
		}
	}
	return choice
}

// theWall is what the pages say of the sandbox, and how a person confirms there is none.
func theWall() web.Wall {
	return web.Wall{
		Choice: *wall,
		Accepted: func() bool {
			_, err := os.Stat(confirmedFile)
			return err == nil
		},
		Accept: func() error { return os.WriteFile(confirmedFile, []byte("agents run with no sandbox\n"), 0o600) },
	}
}
