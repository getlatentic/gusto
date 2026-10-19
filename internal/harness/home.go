package harness

import (
	"maps"
	"regexp"
	"slices"
)

// An adapter is written for a container, where the harness's home is /root and what it keeps between turns is under
// /root/.local/share, the instance's own state. A harness that runs anywhere else is given a home and a data folder of its
// own, and every mention of the two is moved there.
var (
	containerHome = regexp.MustCompile(`(^|[^\w./-])/root($|/)`)
	containerData = regexp.MustCompile(`(^|[^\w./-])/root/\.local/share($|/)`)
)

// Rehome rewrites every mention of the container's home in text to home: as a path of its own, and not inside another path
// such as /usr/root or /rootless.
func Rehome(text, home string) string {
	return containerHome.ReplaceAllString(text, "${1}"+home+"${2}")
}

// At is the adapter for an agent whose home is home and whose data folder is data, rather than /root and
// /root/.local/share: its arguments, environment and folders name them.
func (a Adapter) At(home, data string) Adapter {
	move := func(text string) string {
		return Rehome(containerData.ReplaceAllString(text, "${1}"+data+"${2}"), home)
	}
	moved := func(list []string) []string {
		out := slices.Clone(list)
		for i := range out {
			out[i] = move(out[i])
		}
		return out
	}
	a.Argv, a.Resume = moved(a.Argv), moved(a.Resume)
	a.Env = maps.Clone(a.Env)
	for key, value := range a.Env {
		a.Env[key] = move(value)
	}
	a.Keeps, a.Skills = move(a.Keeps), move(a.Skills)
	return a
}
