package datadir

import "fmt"

// Release is what a published release wrote into each store, so an operator can
// name the release to go back to and not a format number.
type Release struct {
	Name    string
	Formats map[Store]int
}

// Releases is every published release, oldest first. A release that changes a
// format adds itself here in the same change.
var Releases = []Release{
	{Name: "v0.1.0-rc.1", Formats: map[Store]int{
		Jobs: 1, Sessions: 1, Setups: 1, Comparisons: 1, Changes: 1, Tokens: 1, Retention: 1, Integrations: 1,
	}},
	{Name: "v0.0.1-alpha.1", Formats: map[Store]int{
		Jobs: 1, Sessions: 1, Setups: 1, Comparisons: 1, Changes: 1, Tokens: 1, Retention: 1, Integrations: 1,
	}},
	{Name: "v0.0.1-alpha.2", Formats: map[Store]int{
		Jobs: 1, Sessions: 1, Setups: 1, Comparisons: 1, Changes: 1, Tokens: 1, Retention: 1, Integrations: 1,
	}},
	{Name: "v0.0.1-alpha.3", Formats: map[Store]int{
		Jobs: 1, Sessions: 1, Setups: 1, Comparisons: 1, Changes: 1, Tokens: 1, Retention: 1, Integrations: 1,
	}},
	{Name: "v0.0.1-alpha.4", Formats: map[Store]int{
		Jobs: 1, Sessions: 1, Setups: 1, Comparisons: 1, Changes: 1, Tokens: 1, Retention: 1, Integrations: 1,
	}},
	{Name: "v0.0.1-alpha.5", Formats: map[Store]int{
		Jobs: 1, Sessions: 1, Setups: 1, Comparisons: 1, Changes: 1, Tokens: 1, Retention: 1, Integrations: 1,
	}},
}

// FormatsOf is what a named release wrote.
func FormatsOf(name string) (map[Store]int, error) {
	for _, release := range Releases {
		if release.Name == name {
			return release.Formats, nil
		}
	}
	var names []string
	for _, release := range Releases {
		names = append(names, release.Name)
	}
	return nil, fmt.Errorf("%q is not a release this one knows the formats of: %v", name, names)
}
