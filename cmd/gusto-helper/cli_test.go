package main

import (
	"testing"

	"github.com/getlatentic/gusto/cmd/internal/cmdline/clitest"
)

func TestTheHelperKeepsTheCommandLineContract(t *testing.T) {
	clitest.Verify(t, program())
}
