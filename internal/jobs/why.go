package jobs

import (
	"fmt"
	"os"
	"strings"

	"github.com/getlatentic/gusto/pkg/job"
)

// explainFailure leaves the reason a task failed in its own record when the run wrote nothing of its own, so a page that shows the
// task has a line to show. A run that wrote a transcript, an error log or a manifest speaks for itself and is left as it is.
func explainFailure(dir job.Dir, exit int, err error) {
	if said(dir.OutFile("transcript.log")) || said(dir.OutFile("stderr.log")) || said(dir.OutFile("manifest.json")) {
		return
	}
	reason := fmt.Sprintf("the agent stopped with status %d and wrote nothing", exit)
	if err != nil {
		reason = "gusto could not run this task: " + err.Error()
	}
	if os.MkdirAll(dir.Out(), 0o755) != nil {
		return
	}
	_ = os.WriteFile(dir.OutFile("stderr.log"), []byte(reason+"\n"), 0o644)
}

// said reports whether a file holds anything but space.
func said(path string) bool {
	text, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(text)) != ""
}
