package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/cmd/internal/cmdline/clitest"
)

func TestTheCommandReferenceInTheDocsIsTheOneGustoPrints(t *testing.T) {
	generated, _, code := clitest.Capture(t, func() int { return run([]string{"help", "--markdown"}) })
	if code != 0 {
		t.Fatalf("gusto help --markdown: exit code %d", code)
	}
	path := filepath.Join("..", "..", "docs", "CLI.md")
	written, err := os.ReadFile(path)
	if err != nil || string(written) != generated {
		t.Fatalf("docs/CLI.md is not what gusto help --markdown prints (%v). Regenerate it from the repository root:\n"+
			"  CC=/usr/bin/clang go run ./cmd/gusto help --markdown > docs/CLI.md", err)
	}
}
