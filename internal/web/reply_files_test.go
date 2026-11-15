package web

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/roomlog"
)

// patchOf is a patch that adds each file named and deletes each one after a "-".
func patchOf(names ...string) string {
	var patch strings.Builder
	for _, name := range names {
		if gone, deleted := strings.CutPrefix(name, "-"); deleted {
			patch.WriteString("diff --git a/" + gone + " b/" + gone + "\ndeleted file mode 100644\n--- a/" + gone + "\n+++ /dev/null\n@@ -1 +0,0 @@\n-old\n")
			continue
		}
		patch.WriteString("diff --git a/" + name + " b/" + name + "\nnew file mode 100644\n--- /dev/null\n+++ b/" + name + "\n@@ -0,0 +1 @@\n+x\n")
	}
	return patch.String()
}

// aRoomTurnWrote is a room whose agent took one turn that changed the files named, with each file's contents put in the room's
// folder. It gives the room page as alice sees it and the turn as it is drawn live.
func aRoomTurnWrote(t *testing.T, files map[string][]byte, names ...string) (page, live, id string) {
	t.Helper()
	work := &roomWork{patch: patchOf(names...)}
	here, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	here.as(t, alice).ask(t, "POST", thread+"/say", map[string][]string{"said": {"write it up"}})
	work.waitTurns(t, here, 1)
	root := here.server.instances.AgentFolder(id)
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	page = here.as(t, alice).ask(t, "GET", thread, nil).Body.String()
	held, _ := here.server.instances.Get(id)
	latest := here.server.instances.Latest(id)
	drawn, err := here.server.drawItem(held, roomlog.Event{Kind: "turn", Thread: latest.ID, Ref: latest.Turns[0]})
	if err != nil {
		t.Fatal(err)
	}
	return page, drawn.HTML, id
}

func TestAReplyShowsTheFilesItWroteAsChips(t *testing.T) {
	files := map[string][]byte{
		"notes.md":  []byte("# Launch notes\n\nThe first paragraph of the notes.\n"),
		"chart.png": append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 3000)...),
		"site.html": []byte("<html><body><h1>Agent headline</h1><script>alert('from the agent')</script></body></html>"),
	}
	names := []string{"notes.md", "chart.png", "site.html", "-old.txt"}
	for i := range chipsShown {
		name := fmt.Sprintf("extra%d.txt", i)
		files[name] = []byte("x\n")
		names = append(names, name)
	}
	page, live, id := aRoomTurnWrote(t, files, names...)

	for name, drawn := range map[string]string{"the page": page, "a live turn": live} {
		if got := strings.Count(drawn, "data-file-chip="); got != chipsShown+3 {
			t.Errorf("%s draws %d file chips, not %d", name, got, chipsShown+3)
		}
		for _, want := range []string{
			`href="/instances/` + id + `/file?path=notes.md&amp;from=`, `data-file-chip="doc"`, `<span class="truncate">notes.md</span>`,
			`data-file-chip="image"`, `<span class="truncate">chart.png</span>`, `data-file-chip="code"`,
			"Show 3 more files<", `old.txt</span> was deleted`,
		} {
			if !strings.Contains(drawn, want) {
				t.Errorf("%s does not draw %q", name, want)
			}
		}
		for _, never := range []string{"Launch notes", "Agent headline", "from the agent", `title="old.txt"`, "<iframe"} {
			if strings.Contains(drawn, never) {
				t.Errorf("%s draws %q, which a chip never shows", name, never)
			}
		}
	}
}
