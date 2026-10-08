package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// entry is one tool of one server as the job's caller may see it.
type entry struct {
	Server string
	Tool   Tool
	// Name is how the job calls it: the server's name and the tool's, as server/tool.
	Name string
	// Flat is the name a harness that lists every tool takes: letters, digits, hyphens and underscores, at most 64.
	Flat string
}

// Catalogue is the tools of a job's servers that its caller may use. It is read from the servers the first time it is asked for.
type Catalogue struct {
	servers map[string]Conn
	order   []string
	policy  func() Policy
	caller  Caller
	record  *Record

	once sync.Once
	// entries is every tool the servers have, and what the caller may use is worked out from the policy as it is when the tool is
	// asked for, so a change to the policy reaches a job that is already running.
	entries []entry
	byName  map[string]entry
	byFlat  map[string]entry
	broken  map[string]string
}

func newCatalogue(servers []Server, private bool, launch Launcher, policy func() Policy, caller Caller, record *Record) *Catalogue {
	c := &Catalogue{servers: map[string]Conn{}, policy: policy, caller: caller, record: record}
	for _, server := range servers {
		if server.IsProgram() {
			c.servers[server.Name] = StartProgram(server, caller.Job, launch)
		} else {
			c.servers[server.Name] = Dial(server, private)
		}
		c.order = append(c.order, server.Name)
	}
	return c
}

func (c *Catalogue) close() {
	for _, upstream := range c.servers {
		upstream.Close()
	}
}

func (c *Catalogue) load(ctx context.Context) {
	c.once.Do(func() {
		c.byName, c.byFlat, c.broken = map[string]entry{}, map[string]entry{}, map[string]string{}
		for _, name := range c.order {
			tools, err := c.servers[name].Tools(ctx)
			if err != nil {
				c.broken[name] = err.Error()
				continue
			}
			for _, tool := range tools {
				found := entry{Server: name, Tool: tool, Name: name + "/" + tool.Name}
				found.Flat = c.flat(name, tool.Name)
				c.entries = append(c.entries, found)
				c.byName[found.Name], c.byFlat[found.Flat] = found, found
			}
		}
		sort.SliceStable(c.entries, func(a, b int) bool { return c.entries[a].Name < c.entries[b].Name })
	})
}

var notFlat = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// flat is a name that is unique among the tools and a harness takes.
func (c *Catalogue) flat(server, tool string) string {
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
func (c *Catalogue) allows(found entry) (bool, string) {
	return c.policy().Allows(c.caller, found.Server, found.Tool.Name)
}

// visible is the tools the caller may use now.
func (c *Catalogue) visible() []entry {
	out := make([]entry, 0, len(c.entries))
	for _, candidate := range c.entries {
		if allowed, _ := c.allows(candidate); allowed {
			out = append(out, candidate)
		}
	}
	return out
}

// find is the tool a name asks for. Both forms of its name are taken.
func (c *Catalogue) find(name string) (entry, bool) {
	if found, ok := c.byName[name]; ok {
		return found, true
	}
	found, ok := c.byFlat[name]
	return found, ok
}

// search is the tools that match a query, best first. A query with no words matches every tool.
func (c *Catalogue) search(query, server string, limit int) []entry {
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

func (c *Catalogue) tools() []Tool {
	visible := c.visible()
	out := make([]Tool, 0, len(visible))
	for _, candidate := range visible {
		out = append(out, candidate.Tool)
	}
	return out
}
