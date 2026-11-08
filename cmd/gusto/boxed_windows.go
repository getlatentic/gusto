package main

import (
	"errors"
	"os"
)

// inheritable refuses: an agent's forwarder runs in a Linux sandbox, never on Windows.
func inheritable(*os.File) error { return errors.New("a run's forwarder runs in a Linux sandbox") }
