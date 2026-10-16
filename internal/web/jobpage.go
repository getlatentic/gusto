package web

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/spend"
)

// job is one run and everything it left: what it said, what it ran, what it
// changed, whether its gate passed, what its calls to a model cost, and the
// evidence that vouches for all of it.
func (s *Server) job(w http.ResponseWriter, r *http.Request) {
	record, dir, found := s.find(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	ledger, _ := s.spending()
	page := map[string]any{
		"Job": record, "On": s.origin(record), "Run": s.recorded(record, ledger),
		"Title": productName + " — " + record.Brief, "Nav": "harnesses",
	}
	spec, err := job.Read(dir)
	if err == nil {
		page["Spec"] = spec
	}
	if held, found := s.instances.Get(record.Instance); found {
		page["Agent"] = s.agentOf(held)
	}
	if record.Thread != "" {
		if thread, found := s.instances.Thread(record.Instance, record.Thread); found {
			page["Thread"] = thread
			page["ThreadPath"] = threadPath(thread)
			page["Talk"] = s.talkAround(thread, record.ID, ledger)
		}
	}
	page["Steps"] = stepsOf(dir)
	manifest := readManifest(dir)
	drawEvidence(page, record, dir, manifest)
	// A turn in a conversation names no gate, so it has no verdict to be
	// missing; saying the gate never answered would call a finished turn broken.
	if err == nil && len(spec.Gate) == 0 {
		delete(page, "Gate")
	}
	page["Ran"] = ranBy(dir)
	page["Calls"] = callsOf(dir)
	page["Files"] = hashed(listing(dir), manifest)
	page["Proof"] = s.proofOf(record.ID, dir, manifest)
	s.render(w, "job.html", page)
}

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
func ranBy(dir job.Dir) []ran {
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

func callsOf(dir job.Dir) *calls {
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
	shown.Tally, _ = ledger.Job("run")
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
func hashed(names []string, manifest *evidence.Manifest) []hashedFile {
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

func listing(dir job.Dir) []string {
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

// talk is the conversation a turn belongs to, as its page places it: which
// message it was, the ones either side, and what the whole conversation has
// taken, cost and changed so far.
type talk struct {
	Name    string
	Place   int
	Of      int
	Before  string
	After   string
	Seconds int64
	Spent   spend.Tally
	Metered bool
	Changed []string
}

func (s *Server) talkAround(thread instance.Thread, id string, ledger *spend.Ledger) talk {
	around := talk{Name: s.nameOf(thread), Of: len(thread.Turns)}
	seen := map[string]bool{}
	for i, turn := range thread.Turns {
		if turn == id {
			around.Place = i + 1
			if i > 0 {
				around.Before = thread.Turns[i-1]
			}
			if i+1 < len(thread.Turns) {
				around.After = thread.Turns[i+1]
			}
		}
		record, found := s.index.Get(turn)
		if !found {
			continue
		}
		if record.Started != 0 && record.Ended != 0 {
			around.Seconds += record.Ended - record.Started
		}
		if spent, metered := spentBy(ledger, turn); metered {
			around.Metered = true
			around.Spent.USD += spent.USD
			around.Spent.Unmeasured += spent.Unmeasured
			around.Spent.Calls += spent.Calls
		}
		for _, file := range changed(record.Dir(s.index.Root())) {
			if !seen[file] {
				seen[file] = true
				around.Changed = append(around.Changed, file)
			}
		}
	}
	return around
}
