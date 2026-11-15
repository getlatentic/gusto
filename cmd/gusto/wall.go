package main

import (
	"fmt"
	"os"

	"github.com/getlatentic/gusto/internal/isolation"
	"github.com/getlatentic/gusto/internal/settings"
	"github.com/getlatentic/gusto/internal/web"
)

// wall is the choice gusto on a person's own computer made when it started: which sandbox agents run behind. It is nil for a server
// that is not on a person's own computer, whose operator chooses and sees it with gusto doctor.
var wall *isolation.Choice

// confirmedFile is where a person's word that they know there is no sandbox is kept, in gusto's own folder.
const confirmedFile = "no-sandbox-confirmed"

// chooseWall is the strongest sandbox this computer can make, as the settings the rest of gusto then reads: GUSTO_SANDBOX, and for a
// container the runtime that answered, which is podman on a computer that has no docker. A person or an operator who set
// GUSTO_SANDBOX is not second-guessed, and nothing is decided for them.
func chooseWall(lookup func(string) string) (map[string]string, error) {
	var boxes sandboxSettings
	set := settings.New()
	boxes.declare(set)
	if err := set.Load(lookup); err != nil {
		return nil, err
	}
	choice := isolation.Choose(isolation.This(boxes.isolation(set.Raw("GUSTO_SANDBOX"))))
	wall = &choice
	sayWall(choice)
	if choice.Forced {
		return nil, nil
	}
	decided := map[string]string{"GUSTO_SANDBOX": sandboxNames[choice.Kind]}
	if choice.Kind == isolation.Container && set.Raw("GUSTO_CONTAINER") == "" {
		decided["GUSTO_CONTAINER"] = choice.Detail
	}
	return decided, nil
}

// sandboxNames is what GUSTO_SANDBOX calls each kind of sandbox.
var sandboxNames = map[isolation.Kind]string{isolation.VM: "openshell", isolation.Container: "container", isolation.Process: "process", isolation.Nothing: "here"}

// sayWall tells the person which sandbox their agents run behind, and why each stronger one is not there.
func sayWall(choice isolation.Choice) {
	fmt.Println(choice.Words())
	for _, passed := range choice.Passed {
		fmt.Printf("  no %s: %s\n", passed.Kind, passed.Detail)
		if passed.Cause != "" {
			fmt.Printf("      %s\n", passed.Cause)
		}
	}
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
