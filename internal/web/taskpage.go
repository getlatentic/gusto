package web

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/pkg/spend"
	"github.com/getlatentic/gusto/pkg/task"
)

// ran is one thing the harness did, as out/commands.jsonl keeps it.
type ran struct {
	Tool   string          `json:"tool"`
	Title  string          `json:"title"`
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	// At is how much of the transcript had been written when it ran, which puts it among the words around it. A run from before it was kept has none.
	At *int64 `json:"at,omitempty"`
}

// Says is the line a person reads for it: the harness's own title when it
// gave one, and otherwise what it was asked to do.
func (r ran) Says() string {
	if r.Title != "" {
		return r.Title
	}
	var command struct {
		Command string `json:"command"`
		Path    string `json:"file_path"`
	}
	if json.Unmarshal(r.Input, &command) == nil {
		if command.Command != "" {
			return command.Command
		}
		if command.Path != "" {
			return command.Path
		}
	}
	var plain string
	if json.Unmarshal(r.Input, &plain) == nil {
		return plain
	}
	return string(r.Input)
}

// ranBy reads what a run did. It is bounded, because a harness that loops
// can write thousands, and the file itself is one link away.
func ranBy(dir task.Dir) []ran {
	file, err := os.Open(dir.OutFile("commands.jsonl"))
	if err != nil {
		return nil
	}
	defer file.Close()
	const most = 200
	var done []ran
	lines := bufio.NewScanner(file)
	lines.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for lines.Scan() && len(done) < most {
		var one ran
		if json.Unmarshal(lines.Bytes(), &one) == nil {
			done = append(done, one)
		}
	}
	return done
}

// calls is a run's traffic through the proxy: the model calls it made, what
// they came to, and the other hosts it tried to reach.
type calls struct {
	Tally  spend.Tally
	Model  []spend.Call
	Denied []spend.Call
}

// blocked is one site a run was refused, and how often it tried.
type blocked struct {
	Host  string
	Times int
}

// Sites is each refused site once, in the order it was first tried, since a
// harness retrying one address reads as a list of the same line otherwise.
func (c calls) Sites() []blocked {
	var sites []blocked
	at := map[string]int{}
	for _, call := range c.Denied {
		host := strings.TrimSuffix(call.Host, ":443")
		if i, seen := at[host]; seen {
			sites[i].Times++
			continue
		}
		at[host] = len(sites)
		sites = append(sites, blocked{Host: host, Times: 1})
	}
	return sites
}

func callsOf(dir task.Dir) *calls {
	read, err := spend.ReadCalls(dir.OutFile("proxy.jsonl"))
	if err != nil || len(read) == 0 {
		return nil
	}
	ledger := spend.NewLedger(spend.Policy{})
	shown := &calls{}
	for _, call := range read {
		if call.ToModel() {
			shown.Model = append(shown.Model, call)
			ledger.Count("run", call)
		} else if call.Denied != "" {
			shown.Denied = append(shown.Denied, call)
		}
	}
	shown.Tally, _ = ledger.Task("run")
	return shown
}

// hashedFile is one file in out/ beside the hash the manifest claims for it, so
// a reader can hash their own download and compare without fetching the manifest
// as well.
type hashedFile struct {
	Name   string
	SHA256 string
}

// hashed pairs what is on disk with what the manifest says. No hash means the
// manifest does not name the file: manifest.json, which cannot hash itself, and
// anything that appeared after the run, which `gusto verify` reports.
func hashed(names []string, manifest *record.Manifest) []hashedFile {
	claimed := map[string]string{}
	for _, entry := range entriesOf(manifest) {
		claimed[entry.Name] = entry.SHA256
	}
	files := make([]hashedFile, 0, len(names))
	for _, name := range names {
		files = append(files, hashedFile{Name: name, SHA256: claimed[name]})
	}
	return files
}

func listing(dir task.Dir) []string {
	entries, err := os.ReadDir(dir.Out())
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			names = append(names, entry.Name())
		}
	}
	return names
}

// talkAround places a turn in its conversation: which message it was, and the ones either side.
func (s *Server) talkAround(thread instance.Thread, id string) talkView {
	around := talkView{Name: s.nameOf(thread), Of: len(thread.Turns)}
	for i, turn := range thread.Turns {
		if turn != id {
			continue
		}
		around.Place = i + 1
		if i > 0 {
			around.Before = thread.Turns[i-1]
		}
		if i+1 < len(thread.Turns) {
			around.After = thread.Turns[i+1]
		}
	}
	return around
}
