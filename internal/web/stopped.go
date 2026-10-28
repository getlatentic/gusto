package web

import (
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/pkg/job"
)

// noReason is what a failed turn says when nothing it left says why, so a failed turn never shows an empty card.
const noReason = "it ended without saying why"

// stopped is why a turn that failed or was interrupted ended, in one line: the proxy's word that the model never answered
// when the record has it, since that is gusto's own evidence and holds for any agent; otherwise the last line the agent or the
// runner wrote on its way out.
func stopped(dir job.Dir) string {
	if manifest, err := evidence.Read(dir); err == nil && manifest.ModelUnanswered != nil {
		return "the model did not answer: " + manifest.ModelUnanswered.Reason
	}
	for _, name := range []string{"stderr.log", "transcript.log"} {
		if line := lastSaid(dir.OutFile(name)); line != "" {
			return line
		}
	}
	return noReason
}

// mostTailBytes bounds how much of a log is read for its last line: a reason is short, and a log can be large.
const mostTailBytes = 16 << 10

// terminalCodes are the escape sequences an agent writes for a terminal, which say nothing on a page.
var terminalCodes = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]|\x1b\][^\x07]*\x07`)

// lastSaid is the last line of a file with words in it, without terminal codes, or empty.
func lastSaid(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	if info, err := file.Stat(); err == nil && info.Size() > mostTailBytes {
		if _, err := file.Seek(-mostTailBytes, io.SeekEnd); err != nil {
			return ""
		}
	}
	text, err := io.ReadAll(io.LimitReader(file, mostTailBytes))
	if err != nil {
		return ""
	}
	lines := strings.Split(terminalCodes.ReplaceAllString(string(text), ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(strings.Map(printable, lines[i])); line != "" {
			return line
		}
	}
	return ""
}

func printable(r rune) rune {
	if r < ' ' && r != '\t' {
		return -1
	}
	return r
}
