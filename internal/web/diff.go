package web

import (
	"strconv"
	"strings"
)

// The kinds of line a hunk holds. Each is also the CSS class the page draws the
// line with.
const (
	addedLine   = "added"
	removedLine = "removed"
	contextLine = "context"
	noteLine    = "note"
)

// diffFile is one file's section of a patch: the header git writes about it,
// then its hunks.
type diffFile struct {
	Name    string
	Meta    []string
	Binary  bool
	Deleted bool
	Hunks   []hunk
	Added   int
	Removed int
}

type hunk struct {
	Header string
	Lines  []diffLine
}

// diffLine is one line of a hunk with the numbers it has on each side. A zero
// is a side the line is not on.
type diffLine struct {
	Kind string
	Old  int
	New  int
	Text string
}

// parseDiff reads the output of `git diff`, which is what a runner writes to
// diff.patch. It only sorts lines, it never rejects one: the patch comes from a
// harness nobody trusts, and a line it cannot place is still drawn as a note so
// nothing in the evidence is hidden from the reader.
//
// Inside a hunk every line starts with a space, '+', '-' or '\', so a line
// starting "diff " or "@@" can only be a header, and a removed line that reads
// "--- x" cannot be mistaken for the start of a file.
func parseDiff(patch string) []diffFile {
	if patch == "" {
		return nil
	}
	var reader diffReader
	for _, line := range strings.Split(strings.TrimSuffix(patch, "\n"), "\n") {
		reader.read(line)
	}
	return reader.files
}

type diffReader struct {
	files  []diffFile
	inHunk bool
	old    int
	new    int
}

func (r *diffReader) read(line string) {
	switch {
	case strings.HasPrefix(line, "diff "):
		r.startFile(line)
	case strings.HasPrefix(line, "@@"):
		r.startHunk(line)
	case r.inHunk:
		r.hunkLine(line)
	default:
		r.metaLine(line)
	}
}

func (r *diffReader) startFile(header string) {
	r.files = append(r.files, diffFile{Name: gitName(header)})
	r.inHunk = false
}

func (r *diffReader) startHunk(header string) {
	file := r.current()
	file.Hunks = append(file.Hunks, hunk{Header: header})
	r.old, r.new = hunkStart(header)
	r.inHunk = true
}

func (r *diffReader) hunkLine(line string) {
	file := r.current()
	drawn := r.number(classify(line))
	switch drawn.Kind {
	case addedLine:
		file.Added++
	case removedLine:
		file.Removed++
	}
	last := &file.Hunks[len(file.Hunks)-1]
	last.Lines = append(last.Lines, drawn)
}

// number gives a line the numbers it has on each side and moves the count on
// for the sides it is on.
func (r *diffReader) number(line diffLine) diffLine {
	if line.Kind == addedLine || line.Kind == contextLine {
		line.New = r.new
		r.new++
	}
	if line.Kind == removedLine || line.Kind == contextLine {
		line.Old = r.old
		r.old++
	}
	return line
}

// metaLine is anything between a file's first line and its first hunk. The
// "---" and "+++" lines name the file, so they rename the section instead of
// being drawn.
func (r *diffReader) metaLine(line string) {
	file := r.current()
	switch {
	case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "):
		file.rename(line[4:])
	case strings.HasPrefix(line, "Binary files "):
		file.Binary = true
		file.Meta = append(file.Meta, line)
	case strings.HasPrefix(line, "deleted file mode "):
		file.Deleted = true
		file.Meta = append(file.Meta, line)
	default:
		file.Meta = append(file.Meta, line)
	}
}

// current is the section being read. A patch that does not start with a git
// header still gets one, unnamed, so its lines have somewhere to go.
func (r *diffReader) current() *diffFile {
	if len(r.files) == 0 {
		r.files = append(r.files, diffFile{})
	}
	return &r.files[len(r.files)-1]
}

// rename takes a name from a "---" or "+++" line. "+++" comes second, so the
// new name wins, and a deleted file, whose "+++" is /dev/null, keeps its old one.
func (f *diffFile) rename(path string) {
	path, _, _ = strings.Cut(path, "\t")
	if path == "/dev/null" {
		return
	}
	f.Name = stripSide(unquote(path))
}

var kinds = map[byte]string{'+': addedLine, '-': removedLine, ' ': contextLine, '\\': noteLine}

// classify splits a hunk line into its kind and its text. A blank line is a
// context line whose leading space was stripped on the way, which some editors
// and mail clients do.
func classify(line string) diffLine {
	if line == "" {
		return diffLine{Kind: contextLine}
	}
	kind, known := kinds[line[0]]
	if !known || kind == noteLine {
		return diffLine{Kind: noteLine, Text: line}
	}
	return diffLine{Kind: kind, Text: line[1:]}
}

// hunkStart reads the first line number of each side from a header such as
// "@@ -12,7 +12,9 @@ func main() {".
func hunkStart(header string) (int, int) {
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return 0, 0
	}
	return rangeStart(fields[1]), rangeStart(fields[2])
}

func rangeStart(field string) int {
	start, _, _ := strings.Cut(strings.TrimLeft(field, "-+"), ",")
	number, _ := strconv.Atoi(start)
	return number
}

// gitName reads the file's name from "diff --git a/x b/x". A file whose name
// holds " b/" is ambiguous there, so the two halves are taken as equal when
// they can be, which they are for every change but a rename. A binary file or a
// pure rename has no "---" line, so this is the name it keeps.
func gitName(header string) string {
	names := strings.TrimPrefix(strings.TrimPrefix(header, "diff --git "), "diff ")
	if half := len(names) / 2; len(names)%2 == 1 && names[half] == ' ' {
		if left := stripSide(unquote(names[:half])); left == stripSide(unquote(names[half+1:])) {
			return left
		}
	}
	if at := strings.LastIndex(names, " b/"); at >= 0 {
		return names[at+3:]
	}
	return names
}

func stripSide(path string) string {
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}
	return path
}

// unquote reads a name git quoted because it holds a byte git will not print
// bare. Git's quoting is C's, which Go's is close enough to for the octal
// escapes it uses.
func unquote(path string) string {
	if unquoted, err := strconv.Unquote(path); err == nil {
		return unquoted
	}
	return path
}
