package main

import (
	"fmt"
	"os"

	"github.com/getlatentic/haas/internal/isolation"
	"github.com/getlatentic/haas/internal/web"
)

// wall is the choice haas on a person's own computer made when it started: which wall agents run behind. It is nil for a server
// that is not on a person's own computer, whose operator chooses and sees it with haas doctor.
var wall *isolation.Choice

// runsHere says jobs run in this process, behind no wall or behind this computer's own sandbox, and not in a container or a VM.
func runsHere() bool {
	sandbox := os.Getenv("HAAS_SANDBOX")
	return sandbox == "here" || sandbox == "process"
}

// confirmedFile is where a person's word that they know there is no sandbox is kept, in haas's own folder.
const confirmedFile = "no-sandbox-confirmed"

// chooseWall is the strongest wall this computer can make, set as HAAS_SANDBOX so the rest of haas runs jobs behind it. A person or
// an operator who set HAAS_SANDBOX is not second-guessed.
func chooseWall() isolation.Choice {
	choice := isolation.Choose(isolation.This())
	if !choice.Forced {
		os.Setenv("HAAS_SANDBOX", map[isolation.Kind]string{isolation.VM: "openshell", isolation.Container: "container", isolation.Process: "process", isolation.Nothing: "here"}[choice.Kind])
		// The runtime that answered is the one jobs are started with, which is podman on a computer that has no docker.
		if choice.Kind == isolation.Container && os.Getenv("HAAS_CONTAINER") == "" {
			os.Setenv("HAAS_CONTAINER", choice.Detail)
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

// theWall is what the pages say of the wall, and how a person confirms there is none.
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
