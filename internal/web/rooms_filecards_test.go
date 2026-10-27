package web

import (
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
	root := here.server.instances.Workspace(id)
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

func TestARoomShowsTheFilesAReplyWroteAsCards(t *testing.T) {
	notes := []byte("# Launch notes\n\nThe first paragraph of the notes.\n")
	todo := []byte(strings.Repeat("a line to do\n", 30))
	chart := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 3000)...)
	page, live, id := aRoomTurnWrote(t, map[string][]byte{"notes.md": notes, "todo.txt": todo, "chart.png": chart},
		"notes.md", "todo.txt", "chart.png", "-old.txt")

	for name, drawn := range map[string]string{"the page": page, "a live turn": live} {
		if got := strings.Count(drawn, "data-file-card="); got != 3 {
			t.Errorf("%s draws %d file cards, not 3", name, got)
		}
		for _, want := range []string{
			`title="notes.md">notes.md</a>`, `title="todo.txt">todo.txt</a>`, `title="chart.png">chart.png</a>`,
			`/instances/` + id + `/raw?path=notes.md&amp;download=1`, `/instances/` + id + `/raw?path=todo.txt&amp;download=1`,
			`/instances/` + id + `/raw?path=chart.png&amp;download=1`, `/instances/` + id + `/file?path=notes.md`,
			">50 bytes<", ">390 bytes<", ">3 KB<",
			`data-file-card="markdown"`, `data-preview="markdown"`, "<h1>Launch notes</h1>",
			`data-file-card="text"`, `data-preview="text"`, `data-file-card="image"`, `data-preview="image"`,
			`<img class="max-h-full max-w-full rounded-sm object-contain" src="/instances/` + id + `/raw?path=chart.png"`,
			"Show 1 more file<", `old.txt</span> was deleted`,
		} {
			if !strings.Contains(drawn, want) {
				t.Errorf("%s does not draw %q", name, want)
			}
		}
		text := between(drawn, `data-preview="text">`, "</pre>")
		if lines := strings.Count(text, "a line to do"); lines != previewLines {
			t.Errorf("%s previews %d lines of the text file, not %d", name, lines, previewLines)
		}
		if strings.Contains(drawn, `title="old.txt"`) {
			t.Errorf("%s draws a card for a deleted file", name)
		}
	}
}

func TestARoomPreviewsAPageAnAgentWroteOnlyInASealedFrame(t *testing.T) {
	written := []byte("<html><body><h1>Agent headline</h1><script>alert('from the agent')</script></body></html>")
	page, live, id := aRoomTurnWrote(t, map[string][]byte{"site.html": written}, "site.html")
	for name, drawn := range map[string]string{"the page": page, "a live turn": live} {
		frame := between(drawn, `data-preview="html"`, "</iframe>")
		if !strings.Contains(frame, `sandbox=""`) || !strings.Contains(frame, `src="/instances/`+id+`/raw?path=site.html"`) {
			t.Errorf("%s does not frame the page sealed from its own address:\n%s", name, frame)
		}
		if strings.Contains(drawn, "Agent headline") || strings.Contains(drawn, "from the agent") {
			t.Errorf("%s puts the agent's page into the room's own", name)
		}
	}
}

func TestAFileCardReadsNoMoreOfAFileThanTheCap(t *testing.T) {
	big := []byte(strings.Repeat("x", previewBytes+4096) + "PAST-THE-CAP" + strings.Repeat("y", 1<<20))
	page, _, _ := aRoomTurnWrote(t, map[string][]byte{"big.txt": big}, "big.txt")
	if !strings.Contains(page, ">1.0 MB<") {
		t.Error("the card does not give the big file's size")
	}
	text := between(page, `data-preview="text">`, "</pre>")
	if len(text) > previewBytes || strings.Contains(page, "PAST-THE-CAP") {
		t.Errorf("the card previews %d bytes of the file, past the cap of %d", len(text), previewBytes)
	}
}
