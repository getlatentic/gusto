package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sayWith is a message sent from a room's box with files, as the browser's form sends it.
func sayWith(who *site, thread, said string, files map[string]string) *httptest.ResponseRecorder {
	var sent bytes.Buffer
	form := multipart.NewWriter(&sent)
	form.WriteField("said", said)
	for name, body := range files {
		part, _ := form.CreateFormFile("file", name)
		part.Write([]byte(body))
	}
	form.Close()
	request := httptest.NewRequest("POST", thread+"/say", &sent)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.AddCookie(who.cookie)
	answer := httptest.NewRecorder()
	who.handler.ServeHTTP(answer, request)
	return answer
}

func TestAMessageWithFilesKeepsThemInTheRoomAndNamesThemToTheAgent(t *testing.T) {
	work := &roomWork{}
	here, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	mine := here.as(t, alice)
	folder := here.server.instances.AgentFolder(id)
	if err := os.WriteFile(filepath.Join(folder, "report.pdf"), []byte("older"), 0o644); err != nil {
		t.Fatal(err)
	}

	answer := sayWith(mine, thread, "@Scout summarise these", map[string]string{"report.pdf": "newer", "notes.txt": "the notes"})
	if answer.Code != http.StatusSeeOther || strings.Contains(answer.Header().Get("Location"), "refused") {
		t.Fatalf("a message with files answered %d to %s", answer.Code, answer.Header().Get("Location"))
	}
	if kept, _ := os.ReadFile(filepath.Join(folder, "report (2).pdf")); string(kept) != "newer" {
		t.Errorf("a file whose name was taken was kept as %q, and the older one is left as it was", kept)
	}
	if kept, _ := os.ReadFile(filepath.Join(folder, "notes.txt")); string(kept) != "the notes" {
		t.Errorf("notes.txt was kept as %q", kept)
	}
	heard := work.waitTurns(t, here, 1)
	if len(heard) != 1 || !strings.Contains(heard[0].Brief, "@Scout summarise these") || !strings.Contains(heard[0].Brief, "Attached: ") ||
		!strings.Contains(heard[0].Brief, "report (2).pdf") || !strings.Contains(heard[0].Brief, "notes.txt") {
		t.Fatalf("the agent was not told which files came with the message: %+v", heard)
	}
	if page := mine.ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, "Attached: ") || strings.Count(page, "summarise these") != 1 {
		t.Error("the room does not show the files with the message as one turn")
	}
}

func TestAFileCanBeSentWithNoWordsAndOnlyAMemberMaySend(t *testing.T) {
	work := &roomWork{}
	here, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	answer := sayWith(here.as(t, alice), thread, "", map[string]string{"brief.md": "the brief"})
	if answer.Code != http.StatusSeeOther || strings.Contains(answer.Header().Get("Location"), "refused") {
		t.Fatalf("files alone answered %d to %s", answer.Code, answer.Header().Get("Location"))
	}
	if heard := work.waitTurns(t, here, 1); !strings.Contains(heard[0].Brief, "Attached: brief.md") {
		t.Errorf("the agent was told %q", heard[0].Brief)
	}
	if got := sayWith(here.as(t, bob), thread, "hello", map[string]string{"x.txt": "no"}); got.Code != http.StatusNotFound {
		t.Errorf("somebody outside the room sent files to it: %d", got.Code)
	}
	if _, err := os.Stat(filepath.Join(here.server.instances.AgentFolder(id), "x.txt")); err == nil {
		t.Error("a file from somebody outside the room was kept")
	}
}

func TestAFileWhoseNameWouldMentionAnAgentIsRefusedAndNothingIsSent(t *testing.T) {
	work := &roomWork{}
	here, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	answer := sayWith(here.as(t, alice), thread, "have a look", map[string]string{"@Scout.md": "x"})
	if where := answer.Header().Get("Location"); !strings.Contains(where, "refused") {
		t.Fatalf("a file named like a mention was taken: %s", where)
	}
	if _, err := os.Stat(filepath.Join(here.server.instances.AgentFolder(id), "@Scout.md")); err == nil {
		t.Error("the refused file was kept")
	}
	if len(work.turns()) != 0 {
		t.Error("a refused message started a turn")
	}
}

func TestAMessageWithMoreFilesThanAMessageTakesIsRefused(t *testing.T) {
	here, id, thread := aRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	before, _ := os.ReadDir(here.server.instances.AgentFolder(id))
	files := map[string]string{}
	for i := range mostUploads + 1 {
		files[strings.Repeat("a", i+1)+".txt"] = "x"
	}
	answer := sayWith(here.as(t, alice), thread, "all of them", files)
	if where := answer.Header().Get("Location"); !strings.Contains(where, "refused") {
		t.Fatalf("%d files were taken: %s", len(files), where)
	}
	if kept, _ := os.ReadDir(here.server.instances.AgentFolder(id)); len(kept) != len(before) {
		t.Errorf("%d files were kept of a message that was refused", len(kept)-len(before))
	}
}

func TestAPlainMessageIsStillSentAsAFormOrOverTheSocket(t *testing.T) {
	work := &roomWork{}
	here, _, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	answer := sayWith(here.as(t, alice), thread, "@Scout just words", nil)
	if answer.Code != http.StatusSeeOther || strings.Contains(answer.Header().Get("Location"), "refused") {
		t.Fatalf("a message with no files answered %d to %s", answer.Code, answer.Header().Get("Location"))
	}
	if heard := work.waitTurns(t, here, 1); strings.Contains(heard[0].Brief, "Attached") {
		t.Errorf("a message with no files was told to the agent as having some: %s", heard[0].Brief)
	}
}
