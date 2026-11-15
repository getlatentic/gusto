package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// An archived room is kept as it was: nobody adds a file to it or changes one, its files say so, and a file opens to be read
// and not to be changed. Bringing the room back lifts all of it.
func TestAnArchivedRoomTakesNoUploadAndNoEditAndSaysSoOnItsFiles(t *testing.T) {
	here, id, _ := aRoomWithBob(t, &roomWork{})
	mine, his := here.as(t, alice), here.as(t, bob)
	folder := here.server.instances.AgentFolder(id)
	if err := os.WriteFile(filepath.Join(folder, "plan.md"), []byte("draft"), 0o644); err != nil {
		t.Fatal(err)
	}
	upload := func(who *site) int {
		var sent bytes.Buffer
		form := multipart.NewWriter(&sent)
		part, _ := form.CreateFormFile("file", "notes.txt")
		part.Write([]byte("from bob"))
		form.Close()
		request := httptest.NewRequest("POST", "/rooms/"+id+"/files", &sent)
		request.Header.Set("Content-Type", form.FormDataContentType())
		request.AddCookie(who.cookie)
		answer := httptest.NewRecorder()
		who.handler.ServeHTTP(answer, request)
		return answer.Code
	}
	edit := func(who *site) int {
		return who.ask(t, "POST", "/instances/"+id+"/file?path=plan.md", url.Values{"text": {"final"}, "seen": {hashOf("draft")}}).Code
	}

	if files := his.roomFilesOf(t, id); files.Archived {
		t.Fatal("a room that is not archived says its files are")
	}
	opened := func() bool {
		return viewed(t, his, id, "plan.md").File.Editable
	}
	if !opened() {
		t.Fatal("a file of a room that is not archived does not open to be changed")
	}
	mine.ask(t, "POST", "/rooms/"+id+"/archive", url.Values{"archive": {"on"}})

	if got := upload(his); got != http.StatusConflict {
		t.Errorf("an upload to an archived room answered %d", got)
	}
	if got := edit(his); got != http.StatusConflict {
		t.Errorf("an edit of a file of an archived room answered %d", got)
	}
	if _, err := os.Stat(filepath.Join(folder, "notes.txt")); !os.IsNotExist(err) {
		t.Error("an archived room took a file")
	}
	if kept, _ := os.ReadFile(filepath.Join(folder, "plan.md")); string(kept) != "draft" {
		t.Errorf("an archived room's file was changed to %q", kept)
	}
	if held := here.server.instances.Latest(id); len(held.Notes) != 0 {
		t.Errorf("an archived room gained %d notices", len(held.Notes))
	}
	if files := his.roomFilesOf(t, id); !files.Archived {
		t.Error("the room's files do not say the room is archived")
	}
	if opened() {
		t.Error("a file of an archived room opens to be changed")
	}

	mine.ask(t, "POST", "/rooms/"+id+"/archive", url.Values{"archive": {"off"}})
	if got := upload(his); got != http.StatusNoContent {
		t.Errorf("an upload to a room that is back answered %d", got)
	}
	if got := edit(his); got != http.StatusOK {
		t.Errorf("an edit in a room that is back answered %d", got)
	}
}
