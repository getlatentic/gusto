package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A link to a file the agent made opens the file in its folder, from the folder it was working in or from its root; a folder opens the list
// of files; and what is not there, is outside the folder or is an address stays what it is.
func TestALinkToAnAgentsFileOpensTheFile(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := strings.TrimPrefix(startedInstance(t, site), "/instances/")
	workspace := site.server.instances.Workspace(id)
	os.MkdirAll(filepath.Join(workspace, "src", "app"), 0o755)
	os.WriteFile(filepath.Join(workspace, "index.html"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(workspace, "src", "main.go"), []byte("x"), 0o644)
	back := "/instances/" + id + "/threads/1"

	drawn := func(text, dir string) string { return string(site.server.replyIn(text, id, dir, back)) }
	file := func(path string) string { return `href="/instances/` + id + `/file?path=` + path + `&amp;from=` }

	if got := drawn("[the game](index.html)", ""); !strings.Contains(got, file("index.html")) {
		t.Errorf("a relative link: %s", got)
	}
	if got := drawn("[the code](main.go)", "src"); !strings.Contains(got, file("src%2Fmain.go")) {
		t.Errorf("a link relative to the folder it worked in: %s", got)
	}
	if got := drawn("[the code](/job/workspace/src/main.go)", "src/app"); !strings.Contains(got, file("src%2Fmain.go")) {
		t.Errorf("a link from the box's root: %s", got)
	}
	if got := drawn("[src](src)", ""); !strings.Contains(got, `href="/instances/`+id+`/files?from=`) {
		t.Errorf("a link to a folder: %s", got)
	}
	for name, text := range map[string]string{"one that is not there": "[x](nope.md)", "one that leaves the folder": "[x](../../etc/passwd)", "one from the root that leaves it": "[x](/job/workspace/../x)"} {
		if got := drawn(text, ""); strings.Contains(got, `href="/instances/`) || strings.Contains(got, "<a ") {
			t.Errorf("%s was made a link: %s", name, got)
		}
	}
	if got := drawn("[the site](https://example.com/x)", ""); !strings.Contains(got, `href="https://example.com/x"`) || !strings.Contains(got, "noopener") {
		t.Errorf("an address: %s", got)
	}
	if got := drawn("[x](javascript:alert(1))", ""); strings.Contains(got, "javascript:") {
		t.Errorf("a script address was kept: %s", got)
	}
	if got := drawn("saved to /job/workspace/index.html", ""); !strings.Contains(got, "saved to index.html") {
		t.Errorf("the box's path was not taken out of prose: %s", got)
	}
}
