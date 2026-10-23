package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

func foldersAt(t *testing.T, site *site, at string) (shownFolders, int) {
	t.Helper()
	answer := site.ask(t, "GET", "/chat/folders?at="+url.QueryEscape(at), nil)
	var shown shownFolders
	json.Unmarshal(answer.Body.Bytes(), &shown)
	return shown, answer.Code
}

// The explorer shows the person's cloud computer: their agents at the top, and inside one its folders and files.
func TestTheExplorerShowsTheFoldersOfThePersonsCloudComputer(t *testing.T) {
	site := open(t)
	site.signIn(t)
	instance := strings.TrimPrefix(startedInstance(t, site), "/instances/")
	workspace := site.server.instances.Workspace(instance)
	os.MkdirAll(filepath.Join(workspace, "src", "app"), 0o755)
	os.MkdirAll(filepath.Join(workspace, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(workspace, "notes.md"), []byte("x"), 0o644)

	top, code := foldersAt(t, site, "")
	if code != http.StatusOK || len(top.Folders) != 1 || top.Folders[0].At != instance {
		t.Fatalf("the top shows %+v (%d)", top, code)
	}
	inside, _ := foldersAt(t, site, instance)
	if len(inside.Folders) != 1 || inside.Folders[0].Name != "src" || len(inside.Files) != 1 || inside.Files[0].Name != "notes.md" || inside.Files[0].Size != 1 || inside.Harness != "" {
		t.Errorf("an agent's folder shows %+v", inside)
	}
	deeper, _ := foldersAt(t, site, instance+"/src")
	if len(deeper.Folders) != 1 || deeper.Folders[0].At != instance+"/src/app" || len(deeper.Crumbs) != 3 {
		t.Errorf("a folder of it shows %+v", deeper)
	}
}

// Nothing outside an agent's workspace can be named, by a path, a link or somebody else's agent.
func TestTheExplorerNamesNothingOutsideAWorkspace(t *testing.T) {
	site := open(t)
	site.signIn(t)
	instance := strings.TrimPrefix(startedInstance(t, site), "/instances/")
	outside := t.TempDir()
	os.Symlink(outside, filepath.Join(site.server.instances.Workspace(instance), "out"))
	for _, at := range []string{instance + "/..", instance + "/../..", instance + "/out", instance + "/nothing", "nobody"} {
		if _, code := foldersAt(t, site, at); code != http.StatusNotFound {
			t.Errorf("%q answered %d", at, code)
		}
	}
}

func TestAFolderIsMadeInsideAnAgentsFolderAndAtTheTopForANewAgent(t *testing.T) {
	site := open(t)
	site.signIn(t)
	instance := strings.TrimPrefix(startedInstance(t, site), "/instances/")
	if made := site.ask(t, "POST", "/chat/folders", url.Values{"at": {instance}, "name": {"drafts"}}); made.Code != http.StatusOK {
		t.Fatalf("making a folder answered %d", made.Code)
	}
	if info, err := os.Stat(filepath.Join(site.server.instances.Workspace(instance), "drafts")); err != nil || !info.IsDir() {
		t.Error("the folder was not made")
	}
	for _, bad := range []string{"../x", "a/b", ".hidden", ""} {
		if made := site.ask(t, "POST", "/chat/folders", url.Values{"at": {instance}, "name": {bad}}); made.Code != http.StatusBadRequest {
			t.Errorf("the name %q answered %d", bad, made.Code)
		}
	}
	top := site.ask(t, "POST", "/chat/folders", url.Values{"harness": {"fixture"}, "name": {"thesis"}})
	if top.Code != http.StatusOK || !strings.Contains(top.Body.String(), `"at":"`) {
		t.Fatalf("a new top folder answered %d %s", top.Code, top.Body.String())
	}
	if !strings.Contains(site.ask(t, "GET", "/chat/folders", nil).Body.String(), `"name":"thesis"`) {
		t.Error("the new folder is not at the top")
	}
}

// A chat in a folder works in it: the conversation holds the folder and so does each turn's job.
func TestAChatInAFolderOfTheCloudComputerWorksThere(t *testing.T) {
	site := open(t)
	site.signIn(t)
	instance := strings.TrimPrefix(startedInstance(t, site), "/instances/")
	os.MkdirAll(filepath.Join(site.server.instances.Workspace(instance), "src"), 0o755)
	answer := site.ask(t, "POST", "/chat", url.Values{"at": {instance + "/src"}, "said": {"tidy up"}})
	where := answer.Header().Get("Location")
	if !strings.HasPrefix(where, "/instances/"+instance+"/threads/") {
		t.Fatalf("a chat in a folder went to %q", where)
	}
	thread := site.server.instances.Latest(instance)
	if thread.Dir != "src" || len(thread.Turns) != 1 {
		t.Fatalf("the conversation holds %+v", thread)
	}
	record, _ := site.index.Get(thread.Turns[0])
	spec, err := job.Read(record.Dir(site.index.Root()))
	if err != nil || spec.Dir != "src" {
		t.Errorf("the turn's job names %q (%v)", spec.Dir, err)
	}
	if refused := site.ask(t, "POST", "/chat", url.Values{"at": {instance + "/../.."}, "said": {"x"}}); !strings.HasPrefix(refused.Header().Get("Location"), "/chat") {
		t.Errorf("a way out of the workspace went to %q", refused.Header().Get("Location"))
	}
}

// Somebody else's agent has no folder to show or to work in.
func TestAnotherPersonsFoldersAreNotShownOrWorkedIn(t *testing.T) {
	site := open(t)
	instance := strings.TrimPrefix(startedInstance(t, site.as(t, alice)), "/instances/")
	other := site.as(t, bob)
	if _, code := foldersAt(t, other, instance); code != http.StatusNotFound {
		t.Errorf("a stranger was shown the folder: %d", code)
	}
	if top, _ := foldersAt(t, other, ""); len(top.Folders) != 0 {
		t.Errorf("a stranger's top shows %+v", top.Folders)
	}
	answer := other.ask(t, "POST", "/chat", url.Values{"at": {instance}, "said": {"x"}})
	if !strings.HasPrefix(answer.Header().Get("Location"), "/chat") {
		t.Errorf("a stranger's chat went to %q", answer.Header().Get("Location"))
	}
}

// A project made a moment ago has said nothing yet, so the chat is still free to choose its harness, and the explorer does not fix one.
func TestAProjectThatHasNotBeenUsedTakesTheHarnessChosenAfterIt(t *testing.T) {
	two := append(offered(), offered()[0])
	two[1].Name = "second"
	site := openOffering(t, two)
	site.signIn(t)
	made := site.ask(t, "POST", "/chat/folders", url.Values{"name": {"thesis"}, "harness": {"fixture"}})
	var project struct{ At string }
	json.Unmarshal(made.Body.Bytes(), &project)
	if project.At == "" {
		t.Fatalf("no project was made: %d %s", made.Code, made.Body.String())
	}
	if top, _ := foldersAt(t, site, ""); len(top.Folders) != 1 || top.Folders[0].Harness != "" {
		t.Errorf("an unused project fixes a harness: %+v", top.Folders)
	}
	started := site.ask(t, "POST", "/chat", url.Values{"said": {"hello"}, "at": {project.At}, "harness": {"second"}})
	where := started.Header().Get("Location")
	if started.Code != http.StatusSeeOther || !strings.Contains(where, "/threads/") {
		t.Fatalf("the chat answered %d to %q", started.Code, where)
	}
	if got, _ := site.server.instances.Get(project.At); got.Harness != "second" {
		t.Errorf("the project kept %q and not the harness chosen", got.Harness)
	}
	if top, _ := foldersAt(t, site, ""); top.Folders[0].Harness != "second" {
		t.Errorf("once something is said the project fixes its harness: %+v", top.Folders)
	}
}
