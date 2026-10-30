package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func hashOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// A text file opens to be changed with all of it, as it is, and what it was; a file that cannot be changed here does not.
func TestATextFileShownWholeCanBeChangedAndOthersCannot(t *testing.T) {
	site, id, workspace := viewerSite(t)
	os.WriteFile(filepath.Join(workspace, "plan.md"), []byte("\n# Plan <b>\n"), 0o644)
	os.WriteFile(filepath.Join(workspace, "big.txt"), []byte(strings.Repeat("a", mostShown+1)), 0o644)
	os.WriteFile(filepath.Join(workspace, "blob.bin"), []byte{0, 1, 2}, 0o644)

	page := viewed(t, site, id, "plan.md")
	source := regexp.MustCompile(`(?s)<textarea[^>]*name="text"[^>]*>\n(.*?)</textarea>`).FindStringSubmatch(page)
	if !strings.Contains(page, "data-edit>Edit</button>") || source == nil || html.UnescapeString(source[1]) != "\n# Plan <b>\n" {
		t.Fatalf("the file does not open to be changed as it is: %q", source)
	}
	if !strings.Contains(page, `name="seen" value="`+hashOf("\n# Plan <b>\n")+`"`) {
		t.Error("the page does not say what the file was")
	}
	if panel := site.ask(t, "GET", "/instances/"+id+"/file?path=plan.md&panel=1", nil).Body.String(); !strings.Contains(panel, "data-file-edit") {
		t.Error("its tab beside a chat cannot change it")
	}
	for _, name := range []string{"big.txt", "blob.bin"} {
		if strings.Contains(viewed(t, site, id, name), "data-file-edit") {
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
	site, id, workspace := viewerSite(t)
	path := filepath.Join(workspace, "notes", "plan.md")
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
	os.WriteFile(filepath.Join(site.server.instances.Workspace(id), "launch-plan.md"), []byte("draft"), 0o644)
	answer := mine.ask(t, "POST", "/instances/"+id+"/file?path=launch-plan.md", url.Values{"text": {"final"}, "seen": {hashOf("draft")}})
	if answer.Code != http.StatusOK {
		t.Fatalf("saving answered %d: %s", answer.Code, answer.Body)
	}
	page := mine.ask(t, "GET", where, nil).Body.String()
	if !strings.Contains(page, "changed launch-plan.md.") || !strings.Contains(page, `/instances/`+id+`/file?path=launch-plan.md&amp;from=`) {
		t.Errorf("the room does not say the file was changed, with a way to it")
	}
}
