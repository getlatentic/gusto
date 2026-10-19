package audit

import (
	"encoding/json"
	"sort"
)

// proxied is what out/proxy.jsonl says about a task's traffic. The file holds
// more than model calls: a call a spending cap refused, which never left, and
// every request to a host other than the model, which is not a model call.
// Each is counted apart, so a refusal or a package download never reads as
// spend. The lines are read as written rather than as spend.Call, because a
// policy may have put a marker where a number was.
type proxied struct {
	calls        int
	bytes        int64
	tokens       measured[int64]
	usd          measured[float64]
	credentials  []string
	registries   []int64
	refused      int
	hostsReached []string
	hostsDenied  []string
}

func proxyCounts(lines []any) proxied {
	counts := proxied{}
	credentials, reached, denied := set{}, set{}, set{}
	registries := map[int64]bool{}
	for _, line := range lines {
		call, _ := line.(map[string]any)
		switch {
		case present(call["host"]):
			host, _ := call["host"].(string)
			if present(call["denied"]) {
				denied.add(host)
			} else {
				reached.add(host)
			}
		case present(call["refused"]):
			counts.refused++
		default:
			counts.modelCall(call, credentials, registries)
		}
	}
	counts.credentials = credentials.sorted()
	counts.registries = []int64{}
	for version := range registries {
		counts.registries = append(counts.registries, version)
	}
	sort.Slice(counts.registries, func(i, j int) bool { return counts.registries[i] < counts.registries[j] })
	counts.hostsReached, counts.hostsDenied = reached.sorted(), denied.sorted()
	return counts
}

// modelCall counts a call that left the box for the model.
func (p *proxied) modelCall(call map[string]any, credentials set, registries map[int64]bool) {
	p.calls++
	p.bytes += integer(call["bytes"])
	p.tokens.add(call["inputTokens"], json.Number.Int64)
	p.tokens.add(call["outputTokens"], json.Number.Int64)
	p.usd.add(call["usd"], json.Number.Float64)
	if name, ok := call["credential"].(string); ok {
		credentials.add(name)
	}
	if version := integer(call["registry"]); version > 0 {
		registries[version] = true
	}
}

// measured is a sum of values the proxy may not have had. One null among them
// makes the sum unknown rather than smaller, because a partial total reads as
// the whole one.
type measured[T int64 | float64] struct {
	sum      T
	unknown  bool
	redacted bool
}

func (m *measured[T]) add(value any, parse func(json.Number) (T, error)) {
	switch value := value.(type) {
	case json.Number:
		if n, err := parse(value); err == nil {
			m.sum += n
			return
		}
	case string:
		m.redacted = true
		return
	}
	m.unknown = true
}

func (m measured[T]) fact() Fact[T] {
	switch {
	case m.redacted:
		return absent[T](Redacted)
	case m.unknown:
		return absent[T](NotMeasured)
	}
	return known(m.sum)
}

// present says a member is there with a value. A policy's marker is a value,
// so a redacted host still keeps its line out of the model calls.
func present(value any) bool {
	return value != nil && value != ""
}

func integer(value any) int64 {
	number, _ := value.(json.Number)
	n, _ := number.Int64()
	return n
}

type set map[string]bool

func (s set) add(name string) { s[name] = true }

func (s set) sorted() []string {
	names := make([]string, 0, len(s))
	for name := range s {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
