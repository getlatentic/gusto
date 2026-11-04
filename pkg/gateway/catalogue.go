package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// entry is one tool of one server as the job's caller may see it.
type entry struct {
	Server string
	Tool   Tool
	// Name is how the task calls it: the server's name and the tool's, as server/tool.
	Name string
	// Flat is the name an agent that lists every tool takes: letters, digits, hyphens and underscores, at most 64.
	Flat string
}

// Catalog is the tools of a task's servers that its caller may use. It is read from the servers the first time it is asked for,
// and a server that could not be read then is read again when it is next asked for, once retryAfter has passed.
type Catalog struct {
	servers map[string]Conn
	order   []string
	policy  func() Policy
	caller  Caller
	record  *Record
	// withheld are servers the task names and the gateway will not call, and why.
	withheld map[string]string
	now      func() time.Time

	// mu guards what is below. A read of the servers happens outside it, one at a time: reading is a load that has a channel open,
	// which those asking meanwhile wait on.
	mu      sync.Mutex
	loading chan struct{}
	// entries is every tool the servers have, and what the caller may use is worked out from the policy as it is when the tool is
	// asked for, so a change to the policy reaches a job that is already running.
	entries []entry
	byName  map[string]entry
	byFlat  map[string]entry
	// read is when each server was last read; broken is why one that was could not be.
	read   map[string]time.Time
	broken map[string]string
}

// retryAfter is how long a server that could not be read is left before it is read again.
const retryAfter = 30 * time.Second

// loadTime bounds one read of a server's tools. A read is not ended by the request that started it, which a cancelled request
// would otherwise leave the server marked broken for the rest of the task.
const loadTime = 60 * time.Second

func newCatalog(servers []Server, private bool, launch Launcher, policy func() Policy, caller Caller, record *Record, client *http.Client, tokens Tokens) *Catalog {
	c := &Catalog{servers: map[string]Conn{}, policy: policy, caller: caller, record: record, now: time.Now,
		byName: map[string]entry{}, byFlat: map[string]entry{}, read: map[string]time.Time{}, broken: map[string]string{}}
	for _, server := range servers {
		if server.IsProgram() {
			c.servers[server.Name] = StartProgram(server, caller.Job, launch)
		} else if client != nil {
			upstream := DialWith(server, client)
			upstream.tokens = tokens
			c.servers[server.Name] = upstream
		} else {
			upstream := Dial(server, private)
			upstream.tokens = tokens
			c.servers[server.Name] = upstream
		}
		c.order = append(c.order, server.Name)
	}
	return c
}

func (c *Catalog) close() {
	for _, upstream := range c.servers {
		upstream.Close()
	}
}

// load reads the servers that have not been read, and those that could not be and are due again. One load runs at a time; a
// request that comes during one waits for it, or for its own end.
func (c *Catalog) load(ctx context.Context) {
	c.mu.Lock()
	if waiting := c.loading; waiting != nil {
		c.mu.Unlock()
		select {
		case <-waiting:
		case <-ctx.Done():
		}
		return
	}
	due := c.due()
	if len(due) == 0 {
		c.mu.Unlock()
		return
	}
	done := make(chan struct{})
	c.loading = done
	c.mu.Unlock()

	read, stop := context.WithTimeout(context.WithoutCancel(ctx), loadTime)
	defer stop()
	found := make(map[string][]Tool, len(due))
	failed := map[string]string{}
	for _, name := range due {
		tools, err := c.servers[name].Tools(read)
		if err != nil {
			failed[name] = err.Error()
			continue
		}
		found[name] = tools
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	at := c.now()
	for _, name := range due {
		c.read[name] = at
		if why, down := failed[name]; down {
			c.broken[name] = why
			continue
		}
		delete(c.broken, name)
		for _, tool := range found[name] {
			added := entry{Server: name, Tool: tool, Name: name + "/" + tool.Name}
			added.Flat = c.flat(name, tool.Name)
			c.entries = append(c.entries, added)
			c.byName[added.Name], c.byFlat[added.Flat] = added, added
		}
	}
	sort.SliceStable(c.entries, func(a, b int) bool { return c.entries[a].Name < c.entries[b].Name })
	c.loading = nil
	close(done)
}

// due is the servers to read now, in order, under mu.
func (c *Catalog) due() []string {
	var due []string
	for _, name := range c.order {
		last, read := c.read[name]
		_, down := c.broken[name]
		if !read || (down && c.now().Sub(last) >= retryAfter) {
			due = append(due, name)
		}
	}
	return due
}

// down is each server that could not be read, and why, in order.
func (c *Catalog) down() [][2]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out [][2]string
	for _, name := range c.order {
		if why, down := c.broken[name]; down {
			out = append(out, [2]string{name, why})
		}
	}
	return out
}

var notFlat = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// flat is a name that is unique among the tools and a harness takes.
func (c *Catalog) flat(server, tool string) string {
	base := notFlat.ReplaceAllString(server+"__"+tool, "_")
	if len(base) > 60 {
		base = base[:60]
	}
	name := base
	for n := 2; ; n++ {
		if _, taken := c.byFlat[name]; !taken {
			return name
		}
		name = fmt.Sprintf("%s_%d", base, n)
	}
}

// allows says whether the caller may use a tool now, and what to say when not.
func (c *Catalog) allows(found entry) (bool, string) {
	return c.policy().Allows(c.caller, found.Server, found.Tool.Name)
}

// visible is the tools the caller may use now.
func (c *Catalog) visible() []entry {
	c.mu.Lock()
	all := slices.Clone(c.entries)
	c.mu.Unlock()
	out := make([]entry, 0, len(all))
	for _, candidate := range all {
		if allowed, _ := c.allows(candidate); allowed {
			out = append(out, candidate)
		}
	}
	return out
}

// find is the tool a name asks for. Both forms of its name are taken.
func (c *Catalog) find(name string) (entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if found, ok := c.byName[name]; ok {
		return found, true
	}
	found, ok := c.byFlat[name]
	return found, ok
}

// search is the tools that match a query, best first. A query with no words matches every tool.
func (c *Catalog) search(query, server string, limit int) []entry {
	words := strings.Fields(strings.ToLower(query))
	type scored struct {
		entry
		score int
	}
	var found []scored
	for _, candidate := range c.visible() {
		if server != "" && candidate.Server != server {
			continue
		}
		name := strings.ToLower(candidate.Name)
		description := strings.ToLower(candidate.Tool.Description + " " + candidate.Tool.Title)
		score := 1
		for _, word := range words {
			switch {
			case strings.Contains(name, word):
				score += 3
			case strings.Contains(description, word):
				score++
			default:
				score = -1
			}
			if score < 0 {
				break
			}
		}
		if len(words) > 0 && score <= 1 {
			continue
		}
		if score > 0 {
			found = append(found, scored{candidate, score})
		}
	}
	sort.SliceStable(found, func(a, b int) bool { return found[a].score > found[b].score })
	out := make([]entry, 0, limit)
	for _, hit := range found {
		if len(out) == limit {
			break
		}
		out = append(out, hit.entry)
	}
	return out
}

// definitionsTokens is about how many tokens the definitions of tools cost a model that is given them all: four characters to a
// token, which is the rule of thumb and not a count.
func definitionsTokens(tools []Tool) int {
	size := 0
	for _, tool := range tools {
		text, _ := json.Marshal(tool)
		size += len(text)
	}
	return (size + 3) / 4
}

func (c *Catalog) tools() []Tool {
	visible := c.visible()
	out := make([]Tool, 0, len(visible))
	for _, candidate := range visible {
		out = append(out, candidate.Tool)
	}
	return out
}
