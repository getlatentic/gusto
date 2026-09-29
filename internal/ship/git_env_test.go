package ship

import (
	"slices"
	"testing"
)

// The token goes to the forge it was minted for and to no other, so a push to a
// GitHub Enterprise Server sends its header there and not to github.com.
func TestTheTokenHeaderIsScopedToTheForgeItBelongsTo(t *testing.T) {
	for host, want := range map[string]string{
		"":                     "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"https://ghe.example":  "GIT_CONFIG_KEY_0=http.https://ghe.example/.extraheader",
		"https://ghe.example/": "GIT_CONFIG_KEY_0=http.https://ghe.example/.extraheader",
	} {
		env := Git{Token: "tok", Host: host}.env()
		if !slices.Contains(env, want) {
			t.Errorf("host %q: %q is not in the environment", host, want)
		}
	}
	for _, entry := range (Git{}).env() {
		if len(entry) > 15 && entry[:15] == "GIT_CONFIG_KEY_" {
			t.Errorf("with no token the environment still carries %q", entry)
		}
	}
}
