//go:build !windows

package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"
)

// A folder is counted as root in a container that cannot reach the network or write the folder, and what it answers is
// kilobytes.
func TestAFolderIsCountedInAContainerThatCannotWriteIt(t *testing.T) {
	script, log := runtimeThat(t, "    echo '2048\t/measured'")
	c := with(script, "image")
	got, err := c.DiskUse(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != 2048*1024 {
		t.Errorf("counted %d bytes", got)
	}
	text, _ := os.ReadFile(log)
	for _, want := range []string{"--network=none", "--read-only", ":/measured:ro", "--cap-drop=ALL", "--entrypoint du", "-sk /measured"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the container was started without %q: %s", want, text)
		}
	}
}

func TestACountThatGivesNothingUsableIsAnError(t *testing.T) {
	for name, says := range map[string]string{"nothing": "    true", "text": "    echo 'big\t/measured'", "negative": "    echo '-5\t/measured'", "failure": "    exit 3"} {
		script, _ := runtimeThat(t, says)
		if got, err := with(script, "image").DiskUse(context.Background(), t.TempDir()); err == nil {
			t.Errorf("%s: counted %d bytes", name, got)
		}
	}
}
