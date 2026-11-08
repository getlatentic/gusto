package main

import (
	"os"
	"testing"
)

// TestMain lets the test binary stand in for gusto where a task's wrapper runs it as `gusto sandbox-run`.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "sandbox-run":
			os.Exit(sandboxRun(os.Args[2:]))
		case "boxed":
			os.Exit(boxed(os.Args[2:]))
		case "boxed-forward":
			os.Exit(boxedForward(os.Args[2:]))
		case "boxed-sign":
			os.Exit(boxedSign(os.Args[2:]))
		}
	}
	os.Exit(m.Run())
}
