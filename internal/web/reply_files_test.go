package web

import (
	"encoding/json"
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
// folder. It gives the turn as alice's page data has it and as it is sent live, each as JSON too.
func aRoomTurnWrote(t *testing.T, files map[string][]byte, names ...string) (page, live turnView, pageJSON, liveJSON string) {
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
	held, _ := here.server.instances.Get(id)
	latest := here.server.instances.Latest(id)
	page, found := here.as(t, alice).conversation(t, thread).turnIn(latest.Turns[0])
	if !found {
		t.Fatal("the page does not have the turn")
	}
	drawn, err := here.server.drawItem(held, roomlog.Event{Kind: "turn", Thread: latest.ID, Ref: latest.Turns[0]})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(drawn.Turn, &live); err != nil {
		t.Fatal(err)
	}
	shown, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	return page, live, string(shown), string(drawn.Turn)
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
	page, live, pageJSON, liveJSON := aRoomTurnWrote(t, files, names...)

	for name, turn := range map[string]turnView{"the page": page, "a live turn": live} {
		chips := turn.Files
		if chips == nil || len(chips.Shown) != chipsShown || len(chips.More) != 3 {
			t.Fatalf("%s has the chips %+v, not %d shown and 3 more", name, chips, chipsShown)
		}
		looks := map[string]string{}
		for _, chip := range append(chips.Shown, chips.More...) {
			looks[chip.Name] = chip.Look
		}
		if looks["notes.md"] != "doc" || looks["chart.png"] != "image" || looks["site.html"] != "code" {
			t.Errorf("%s gives the chips the looks %v", name, looks)
		}
		if len(chips.Removed) != 1 || chips.Removed[0].Path != "old.txt" || chips.Removed[0].Gone != "was deleted" {
			t.Errorf("%s says of the deleted file %+v", name, chips.Removed)
		}
	}
	for name, drawn := range map[string]string{"the page": pageJSON, "a live turn": liveJSON} {
		for _, never := range []string{"Launch notes", "Agent headline", "from the agent"} {
			if strings.Contains(drawn, never) {
				t.Errorf("%s carries %q, which a chip never shows", name, never)
			}
		}
	}
}
