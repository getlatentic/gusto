package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// readDocument reads a JSON file as it is written, without a Go type in the
// way, so a value a policy replaced with a marker is read as the marker.
func readDocument(file string) (any, error) {
	text, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	reader := json.NewDecoder(bytes.NewReader(text))
	reader.UseNumber()
	var value any
	err = reader.Decode(&value)
	return value, err
}

// readLines reads a JSON-lines file. A line that is not JSON is kept as the
// text it is, because leaving it out would hide that it was there.
func readLines(file string) ([]any, error) {
	text, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	lines := []any{}
	scanner := bufio.NewScanner(bytes.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if line := scanner.Bytes(); len(bytes.TrimSpace(line)) > 0 {
			lines = append(lines, lineValue(line))
		}
	}
	return lines, scanner.Err()
}

func lineValue(line []byte) any {
	reader := json.NewDecoder(bytes.NewReader(line))
	reader.UseNumber()
	var value any
	if err := reader.Decode(&value); err != nil {
		return string(line)
	}
	return value
}

func missing(err error) bool { return errors.Is(err, fs.ErrNotExist) }

// changedFiles reads the paths a git patch touches from its headers. The
// b-side is the name after the change, which is what a reviewer looks for.
func changedFiles(patch []byte) []string {
	seen := map[string]bool{}
	for _, line := range strings.Split(string(patch), "\n") {
		header, found := strings.CutPrefix(line, "diff --git ")
		if !found {
			continue
		}
		if at := strings.LastIndex(header, " b/"); at >= 0 {
			seen[header[at+3:]] = true
		}
	}
	files := make([]string, 0, len(seen))
	for name := range seen {
		files = append(files, name)
	}
	sort.Strings(files)
	return files
}

// proxied is what out/proxy.jsonl says left the box: one line a call, written
// by the proxy and not by the harness.
type proxied struct {
	calls       int
	bytes       int64
	credentials []string
}

func proxyCounts(lines []any) proxied {
	counts := proxied{credentials: []string{}}
	named := map[string]bool{}
	for _, line := range lines {
		call, _ := line.(map[string]any)
		counts.calls++
		if size, ok := call["bytes"].(json.Number); ok {
			sent, _ := size.Int64()
			counts.bytes += sent
		}
		if name, ok := call["credential"].(string); ok && !named[name] {
			named[name] = true
			counts.credentials = append(counts.credentials, name)
		}
	}
	sort.Strings(counts.credentials)
	return counts
}

// member reads one field of a document that may not be an object at all.
func member(document any, name string) (any, bool) {
	object, ok := document.(map[string]any)
	if !ok {
		return nil, false
	}
	value, found := object[name]
	return value, found
}
