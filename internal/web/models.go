package web

// Models is what a deployment offers to think with, by harness.
//
// Not every credential answers every harness: a provider can refuse what one
// harness sends while serving another perfectly well. A harness the deployment
// has said nothing about is offered everything, so this only narrows where
// somebody found out that it had to.
type Models map[string][]string

// Everything is the list a deployment offers to whatever asks, which is what a
// deployment that has found no exceptions has.
func Everything(models []string) Models { return Models{"": models} }

// For is what this harness may be pointed at. A harness with no entry of its
// own is offered what everything is offered.
func (m Models) For(harness string) []string {
	if named, found := m[harness]; found {
		return named
	}
	return m[""]
}

// First is what a harness is given when nobody chose, which is what starting
// one from the catalog does. It is empty when the deployment names no model,
// and the harness falls back to its own default.
func (m Models) First(harness string) string {
	if offered := m.For(harness); len(offered) > 0 {
		return offered[0]
	}
	return ""
}
