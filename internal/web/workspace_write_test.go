package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sum(text string) string { h := sha256.Sum256([]byte(text)); return hex.EncodeToString(h[:]) }

// put sends a file with a person's token, and what the writer says it saw.
func (s *site) put(secret, instance, path, body string, saw string, extra ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("PUT", "/api/instances/"+instance+"/file?path="+path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+secret)
	if saw != "" {
		request.Header.Set("X-If-Hash", saw)
	}
	for i := 0; i+1 < len(extra); i += 2 {
		request.Header.Set(extra[i], extra[i+1])
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

func (s *site) remove(secret, instance, path, saw string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("DELETE", "/api/instances/"+instance+"/file?path="+path, nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	if saw != "" {
		request.Header.Set("X-If-Hash", saw)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

func aFolder(t *testing.T) (here *tokened, instance, secret, root string) {
	t.Helper()
	here = withTokens(t)
	where, _ := started(t, here.as(t, alice))
	instance = strings.TrimPrefix(where, "/instances/")
	return here, instance, signInADevice(t, here, alice), here.server.instances.Workspace(instance)
}

func TestAFileIsWrittenWholeWithItsHashAndFoldersAreMadeForIt(t *testing.T) {
	here, id, ada, root := aFolder(t)
	got := here.put(ada, id, "src/deep/new.txt", "hello", absent)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), sum("hello")) {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	if text, err := os.ReadFile(filepath.Join(root, "src/deep/new.txt")); err != nil || string(text) != "hello" {
		t.Errorf("%q %v", text, err)
	}
	left, _ := filepath.Glob(filepath.Join(root, "src/deep/.haas-write-*"))
	if len(left) != 0 {
		t.Errorf("a temporary file was left: %v", left)
	}
	if info, _ := os.Stat(filepath.Join(root, "src/deep/new.txt")); info.Mode().Perm()&0o111 != 0 {
		t.Errorf("a plain file is executable: %v", info.Mode())
	}
	here.put(ada, id, "run.sh", "#!/bin/sh", absent, "X-Mode", "755")
	if info, _ := os.Stat(filepath.Join(root, "run.sh")); info.Mode().Perm()&0o100 == 0 {
		t.Errorf("an executable file lost its bit: %v", info.Mode())
	}
}

func TestAWriteIsRefusedWhenTheFileIsNotWhatTheWriterSawAndNothingChanges(t *testing.T) {
	here, id, ada, root := aFolder(t)
	os.WriteFile(filepath.Join(root, "notes.md"), []byte("the agent's edit"), 0o644)
	if got := here.put(ada, id, "notes.md", "the person's edit", sum("what the person last saw")); got.Code != http.StatusPreconditionFailed || !strings.Contains(got.Body.String(), sum("the agent's edit")) {
		t.Errorf("a write over a file that changed gave %d %s", got.Code, got.Body)
	}
	if got := here.put(ada, id, "notes.md", "x", absent); got.Code != http.StatusPreconditionFailed {
		t.Errorf("a write that says the file is absent, over one that is there, gave %d", got.Code)
	}
	if text, _ := os.ReadFile(filepath.Join(root, "notes.md")); string(text) != "the agent's edit" {
		t.Errorf("the agent's file was changed by a refused write: %q", text)
	}
	if got := here.put(ada, id, "notes.md", "the person's edit", sum("the agent's edit")); got.Code != http.StatusOK {
		t.Errorf("a write over the file the writer saw gave %d", got.Code)
	}
	if got := here.remove(ada, id, "notes.md", sum("stale")); got.Code != http.StatusPreconditionFailed {
		t.Errorf("a delete of a file that changed gave %d", got.Code)
	}
	if got := here.remove(ada, id, "notes.md", sum("the person's edit")); got.Code != http.StatusNoContent {
		t.Errorf("a delete of the file the writer saw gave %d", got.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.md")); err == nil {
		t.Error("the file was not deleted")
	}
	if got := here.remove(ada, id, "notes.md", absent); got.Code != http.StatusNoContent {
		t.Errorf("deleting what is already gone, and was seen gone, gave %d", got.Code)
	}
}

func TestAPathThatLeavesTheFolderOrIsNotPlainIsRefusedAndNothingIsWritten(t *testing.T) {
	here, id, ada, root := aFolder(t)
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(root, "link"))
	os.WriteFile(filepath.Join(outside, "target.txt"), []byte("not yours"), 0o644)
	os.Symlink(filepath.Join(outside, "target.txt"), filepath.Join(root, "linkfile"))
	os.WriteFile(filepath.Join(root, "afile"), []byte("x"), 0o644)
	for _, path := range []string{
		"../escape.txt", "a/../../escape.txt", "/etc/passwd", ".git/config", "src/.git/hooks/x", "a//b", "./x", "a/./b", "..", "",
		"back\\slash", "nul%00byte", "link/new.txt", "linkfile", "afile/inside.txt",
	} {
		if got := here.put(ada, id, path, "bad", ""); got.Code != http.StatusBadRequest {
			t.Errorf("writing %q gave %d %s", path, got.Code, got.Body)
		}
		if got := here.remove(ada, id, path, ""); got.Code != http.StatusBadRequest {
			t.Errorf("deleting %q gave %d", path, got.Code)
		}
	}
	if text, _ := os.ReadFile(filepath.Join(outside, "target.txt")); string(text) != "not yours" {
		t.Errorf("a file outside the folder was changed through a link: %q", text)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); err == nil {
		t.Error("a file was written outside the folder")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 1 {
		t.Errorf("something was made outside the folder: %v", entries)
	}
}

func TestSomebodyElsesFolderCannotBeWrittenOrAskedAboutAndAHugeFileIsRefused(t *testing.T) {
	here, id, ada, root := aFolder(t)
	ben := signInADevice(t, here, bob)
	if got := here.put(ben, id, "x.txt", "no", ""); got.Code != http.StatusNotFound {
		t.Errorf("bob wrote in alice's folder: %d", got.Code)
	}
	if got := here.remove(ben, id, "x.txt", ""); got.Code != http.StatusNotFound {
		t.Errorf("bob deleted in alice's folder: %d", got.Code)
	}
	if got := here.postAs(ben, "/api/instances/"+id+"/hashes", `{"paths":["x"]}`); got.Code != http.StatusNotFound {
		t.Errorf("bob asked about alice's files: %d", got.Code)
	}
	if got := here.put(here.secrets["everyone"], id, "x.txt", "no", ""); got.Code == http.StatusOK {
		t.Error("an organisation's token wrote in a person's folder")
	}
	big := strings.Repeat("x", mostFileBytes+1)
	if got := here.put(ada, id, "big.bin", big, absent); got.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("a file over the limit gave %d", got.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "big.bin")); err == nil {
		t.Error("a file over the limit was kept")
	}
	if left, _ := filepath.Glob(filepath.Join(root, ".haas-write-*")); len(left) != 0 {
		t.Errorf("a refused write left %v", left)
	}
}

func TestHashesAreGivenForFilesAndFollowAFileThatChangesEvenToTheSameSize(t *testing.T) {
	here, id, ada, root := aFolder(t)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("one"), 0o644)
	ask := func() map[string]string {
		got := here.postAs(ada, "/api/instances/"+id+"/hashes", `{"paths":["a.txt","missing.txt","../x",".git/config"]}`)
		var out struct{ Hashes map[string]string }
		json.Unmarshal(got.Body.Bytes(), &out)
		return out.Hashes
	}
	first := ask()
	if len(first) != 1 || first["a.txt"] != sum("one") {
		t.Fatalf("%v", first)
	}
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("two"), 0o644)
	os.Chtimes(filepath.Join(root, "a.txt"), time.Now().Add(time.Second), time.Now().Add(time.Second))
	if second := ask(); second["a.txt"] != sum("two") {
		t.Errorf("the hash did not follow the file: %v", second)
	}
}
