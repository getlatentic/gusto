package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hashOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// A text file opens to be changed with all of it, as it is, and what it was; a file that cannot be changed here does not.
func TestATextFileShownWholeCanBeChangedAndOthersCannot(t *testing.T) {
	site, id, folder := viewerSite(t)
	os.WriteFile(filepath.Join(folder, "plan.md"), []byte("\n# Plan <b>\n"), 0o644)
	os.WriteFile(filepath.Join(folder, "big.txt"), []byte(strings.Repeat("a", mostShown+1)), 0o644)
	os.WriteFile(filepath.Join(folder, "blob.bin"), []byte{0, 1, 2}, 0o644)

	page := viewed(t, site, id, "plan.md")
	if !page.File.Editable || page.File.Source != "\n# Plan <b>\n" || page.Save != "/instances/"+id+"/file?path=plan.md" {
		t.Fatalf("the file does not open to be changed as it is: %+v", page)
	}
	if page.File.Hash != hashOf("\n# Plan <b>\n") {
		t.Error("the page does not say what the file was")
	}
	for _, name := range []string{"big.txt", "blob.bin"} {
		if shown := viewed(t, site, id, name); shown.File.Editable || shown.Save != "" || shown.File.Source != "" {
			t.Errorf("%s opens to be changed", name)
		}
	}
}

func saved(t *testing.T, site *site, id, name, text, seen string) (int, map[string]string) {
	t.Helper()
	answer := site.ask(t, "POST", "/instances/"+id+"/file?path="+url.QueryEscape(name), url.Values{"text": {text}, "seen": {seen}})
	var body map[string]string
	json.Unmarshal(answer.Body.Bytes(), &body)
	return answer.Code, body
}

// A save is put in place when the file is what the person opened, and refused when it changed since, leaving the change there.
func TestASaveIsRefusedWhenTheFileChangedSinceItWasOpened(t *testing.T) {
	site, id, folder := viewerSite(t)
	path := filepath.Join(folder, "notes", "plan.md")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("first"), 0o644)

	code, body := saved(t, site, id, "notes/plan.md", "second", hashOf("first"))
	if got, _ := os.ReadFile(path); code != http.StatusOK || string(got) != "second" || body["hash"] != hashOf("second") {
		t.Fatalf("a save of what was opened: %d %v, the file says %q", code, body, got)
	}
	os.WriteFile(path, []byte("an agent's"), 0o644)
	code, body = saved(t, site, id, "notes/plan.md", "third", hashOf("second"))
	if got, _ := os.ReadFile(path); code != http.StatusConflict || string(got) != "an agent's" || !strings.Contains(body["error"], "changed this file after you opened it") {
		t.Errorf("a save over a change it did not see: %d %v, the file says %q", code, body, got)
	}
	if code, _ := saved(t, site, id, "notes/plan.md", "fourth", ""); code != http.StatusBadRequest {
		t.Errorf("a save that does not say what it saw answered %d", code)
	}
	for _, name := range []string{"../outside.md", "notes/missing.md", "notes"} {
		if code, _ := saved(t, site, id, name, "x", hashOf("x")); code != http.StatusNotFound {
			t.Errorf("saving %s answered %d", name, code)
		}
	}
}

// In a room, a person's change is said in the room with the file it is about, so the people there and its agents know.
func TestAChangeToARoomsFileIsSaidInTheRoom(t *testing.T) {
	site, id, where := aRoom(t, &roomWork{})
	mine := site.as(t, alice)
	os.WriteFile(filepath.Join(site.server.instances.AgentFolder(id), "launch-plan.md"), []byte("draft"), 0o644)
	answer := mine.ask(t, "POST", "/instances/"+id+"/file?path=launch-plan.md", url.Values{"text": {"final"}, "seen": {hashOf("draft")}})
	if answer.Code != http.StatusOK {
		t.Fatalf("saving answered %d: %s", answer.Code, answer.Body)
	}
	page := mine.conversation(t, where)
	if len(page.Turns) != 1 || page.Turns[0].Kind != "notice" || !strings.Contains(page.Turns[0].Said, "changed launch-plan.md.") || page.Turns[0].File != "launch-plan.md" {
		t.Errorf("the room does not say the file was changed, with a way to it: %+v", page.Turns)
	}
}
