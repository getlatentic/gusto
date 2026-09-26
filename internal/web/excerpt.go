package web

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/getlatentic/haas/internal/job"
)

// drawn is how much of one evidence file the job page draws. A harness can
// write a transcript or a patch of any size, and a page that inlines all of it
// stops being readable long before it stops loading. Past this the page links
// the raw file, which the server hands over whole.
const drawn = 256 << 10

// excerpt is the start of one file in out/, cut at a line so no line is drawn
// half way.
type excerpt struct {
	Name  string
	Raw   string
	Text  string
	Size  int64
	Drawn int64
	Cut   bool
}

// readExcerpt reads at most limit bytes of one evidence file. A file that is
// not there is not an error: a run that stopped early leaves some of them
// unwritten, and the page draws what exists.
func readExcerpt(dir job.Dir, id, name string, limit int) (excerpt, bool) {
	file, err := os.Open(dir.OutFile(name))
	if err != nil {
		return excerpt{}, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return excerpt{}, false
	}
	head, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return excerpt{}, false
	}
	cut := len(head) > limit
	if cut {
		head = atLine(head[:limit])
	}
	return excerpt{
		Name: name,
		Raw:  "/jobs/" + id + "/files/" + name,
		// The harness is untrusted and need not write UTF-8. The template
		// escapes markup either way; this keeps a stray byte from turning the
		// rest of a line into mojibake.
		Text:  strings.ToValidUTF8(string(head), "\uFFFD"),
		Size:  max(info.Size(), int64(len(head))),
		Drawn: int64(len(head)),
		Cut:   cut,
	}, true
}

// atLine drops a trailing partial line. One very long line has no line break
// to cut at, so it is kept cut mid-way rather than dropped entirely.
func atLine(head []byte) []byte {
	if last := bytes.LastIndexByte(head, '\n'); last >= 0 {
		return head[:last+1]
	}
	return head
}

// Empty is a file that exists and says nothing, which for a patch is the
// answer "nothing changed".
func (e excerpt) Empty() bool { return e.Size == 0 }

// Whole is the file's size as a person reads it.
func (e excerpt) Whole() string { return measure(e.Size) }

// Shown is how much of the file the page draws.
func (e excerpt) Shown() string { return measure(e.Drawn) }

func measure(size int64) string {
	switch {
	case size >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(size)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", size)
}
