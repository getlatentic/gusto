package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// The install scripts read the version from standard output to say what they installed.
func TestTheVersionIsWrittenToStandardOutput(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = write
	code := versionCommand(nil)
	os.Stdout = stdout
	write.Close()
	said, _ := io.ReadAll(read)
	if code != 0 || !strings.HasPrefix(string(said), "haas ") {
		t.Errorf("haas version wrote %q to standard output and ended %d", said, code)
	}
}
