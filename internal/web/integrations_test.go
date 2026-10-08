package web

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/integration"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/setup"
)

const (
	pluginHead = `"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"`
	mcpHead    = `"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"`
)

func pluginFiles(label, description string) map[string]string {
	return map[string]string{
		"plugin.json":               `{` + pluginHead + `, "name": "reports", "version": "` + label + `"}`,
		"skills/summarize/SKILL.md": "---\nname: summarize\ndescription: " + description + "\n---\n\nSummarise.\n",
		"mcp.json":                  `{` + mcpHead + `, "mcpServers": {"api": {"type": "streamable-http", "url": "https://api.example.com/mcp"}}}`,
	}
}

func zipped(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for name, text := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		file.Write([]byte(text))
	}
	writer.Close()
	return out.Bytes()
}

// upload sends an archive to the import page, as the browser's form does.
func upload(t *testing.T, as *site, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("kind", "upload")
	file, _ := form.CreateFormFile("archive", "reports.zip")
	file.Write(data)
	form.Close()
	request := httptest.NewRequest("POST", "/admin/integrations/import", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.AddCookie(as.cookie)
	recorder := httptest.NewRecorder()
	as.handler.ServeHTTP(recorder, request)
	return recorder
}

func withIntegrations(t *testing.T) (*tokened, *site) {
	t.Helper()
	here := everything(t)
	return here, here.as(t, boss)
}

func TestAnAdministratorImportsAnIntegrationAndItIsOffUntilTurnedOn(t *testing.T) {
	here, admin := withIntegrations(t)
	made := upload(t, admin, zipped(t, pluginFiles("1.0.0", "Summarise a report.")))
	if made.Code != http.StatusSeeOther || !strings.HasPrefix(made.Header().Get("Location"), "/admin/integrations/reports?version=") {
		t.Fatalf("importing answered %d to %q", made.Code, made.Header().Get("Location"))
	}
	held, err := here.server.integrations.Get("reports")
	if err != nil || held.On != "" || len(held.Versions) != 1 {
		t.Fatalf("%+v %v", held, err)
	}
	page := admin.ask(t, "GET", made.Header().Get("Location"), nil).Body.String()
	for _, want := range []string{"summarize", "api.example.com", "off", "Turn on"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not say %q", want)
		}
	}
	if list := admin.ask(t, "GET", "/admin/integrations", nil).Body.String(); !strings.Contains(list, "reports") {
		t.Error("the list does not show it")
	}
}

func TestAJobWithASetupThatNamesAnIntegrationGetsItOnlyOnceItIsOn(t *testing.T) {
	here, admin := withIntegrations(t)
	made := upload(t, admin, zipped(t, pluginFiles("1.0.0", "Summarise a report.")))
	version := strings.SplitN(made.Header().Get("Location"), "version=", 2)[1]
	admin.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"team"}, "integration": {"reports"}})
	if held, _ := here.server.setups.Get("team"); len(held.Integrations) != 1 {
		t.Fatalf("the setup does not name it: %+v", held)
	}
	if page := admin.ask(t, "GET", "/admin/skill-sets/team", nil).Body.String(); !strings.Contains(page, "an administrator turns it on") {
		t.Error("the setup's page does not say the integration is off")
	}

	started := admin.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "setup": {"team"}})
	where := started.Header().Get("Location")
	said := admin.ask(t, "POST", where+"/say", url.Values{"said": {"go"}})
	if strings.Contains(said.Header().Get("Location"), "#") && !strings.Contains(said.Header().Get("Location"), "refused=") {
		id := strings.TrimPrefix(said.Header().Get("Location"), where+"#")
		admin.wait(t, id)
		if spec, err := job.Read(job.Dir(filepath.Join(here.index.Root(), id))); err == nil && spec.Setup != nil && len(spec.Setup.Skills) > 0 {
			t.Fatalf("a job got the skills of an integration that is off: %+v", spec.Setup)
		}
	}

	if on := admin.ask(t, "POST", "/admin/integrations/reports/enable", url.Values{"version": {version}}); on.Code != http.StatusSeeOther {
		t.Fatalf("enabling answered %d", on.Code)
	}
	started = admin.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "setup": {"team"}})
	where = started.Header().Get("Location")
	said = admin.ask(t, "POST", where+"/say", url.Values{"said": {"go"}})
	id := strings.TrimPrefix(said.Header().Get("Location"), where+"#")
	admin.wait(t, id)
	dir := job.Dir(filepath.Join(here.index.Root(), id))
	spec, err := job.Read(dir)
	if err != nil || spec.Setup == nil || strings.Join(spec.Setup.Skills, ",") != "summarize" || strings.Join(spec.Setup.Gateway, ",") != "reports-api" || len(spec.Setup.Servers) != 0 {
		t.Fatalf("job.json names %+v (%v)", spec.Setup, err)
	}
	if use := spec.Setup.Integrations; len(use) != 1 || use[0].Name != "reports" || use[0].Version != version {
		t.Errorf("job.json does not say which version: %+v", use)
	}
	if _, err := os.Stat(filepath.Join(dir.SetupDir(), "skills", "summarize", "SKILL.md")); err != nil {
		t.Error("the skill did not travel with the job")
	}
}

func TestEveryChangeToAnIntegrationIsRecordedWithWhatItWasAndBecame(t *testing.T) {
	here, admin := withIntegrations(t)
	one := upload(t, admin, zipped(t, pluginFiles("1.0.0", "Summarise a report.")))
	two := upload(t, admin, zipped(t, pluginFiles("1.1.0", "Summarise it better.")))
	first, second := strings.SplitN(one.Header().Get("Location"), "version=", 2)[1], strings.SplitN(two.Header().Get("Location"), "version=", 2)[1]
	admin.ask(t, "POST", "/admin/integrations/reports/enable", url.Values{"version": {first}})
	admin.ask(t, "POST", "/admin/integrations/reports/enable", url.Values{"version": {second}})
	admin.ask(t, "POST", "/admin/integrations/reports/restore", nil)
	admin.ask(t, "POST", "/admin/integrations/reports/disable", nil)

	events := written(t, here.log)
	if len(ofKind(events, changelog.IntegrationImported)) != 2 {
		t.Errorf("imports recorded: %d", len(ofKind(events, changelog.IntegrationImported)))
	}
	enabled := ofKind(events, changelog.IntegrationEnabled)
	if len(enabled) != 2 || enabled[1].What["from"] != first || enabled[1].What["to"] != second || !strings.Contains(enabled[1].What["changes"], "skills changed: summarize") {
		t.Errorf("enabled = %+v", enabled)
	}
	restored := ofKind(events, changelog.IntegrationRestored)
	if len(restored) != 1 || restored[0].What["from"] != second || restored[0].What["to"] != first {
		t.Errorf("restored = %+v", restored)
	}
	if off := ofKind(events, changelog.IntegrationDisabled); len(off) != 1 || off[0].What["from"] != first || off[0].What["to"] != "" {
		t.Errorf("disabled = %+v", off)
	}
}

func TestAPluginThatCannotBeTakenIsRefusedOnThePageAndAnUploadOverFourMegabytesIsNotCutOff(t *testing.T) {
	here, admin := withIntegrations(t)
	refused := upload(t, admin, zipped(t, map[string]string{"skills/a/SKILL.md": "x"}))
	if !strings.Contains(refused.Header().Get("Location"), "refused=") {
		t.Errorf("a folder with no plugin.json answered %q", refused.Header().Get("Location"))
	}
	big := pluginFiles("1.0.0", "Summarise a report.")
	random := make([]byte, 5<<20)
	rand.New(rand.NewSource(1)).Read(random)
	big["assets/blob.bin"] = string(random)
	if made := upload(t, admin, zipped(t, big)); !strings.HasPrefix(made.Header().Get("Location"), "/admin/integrations/reports") {
		t.Errorf("a 5 MiB upload answered %q", made.Header().Get("Location"))
	}
	if all, _ := here.server.integrations.List(); len(all) != 1 {
		t.Errorf("integrations kept: %d", len(all))
	}
}

func TestAnIntegrationCanBeImportedFromAGitAddress(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	_, admin := withIntegrations(t)
	repo := t.TempDir()
	for name, text := range pluginFiles("3.0.0", "From git.") {
		full := filepath.Join(repo, name)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(text), 0o644)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "x"}} {
		command := exec.Command("git", args...)
		command.Dir = repo
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
	}
	made := admin.ask(t, "POST", "/admin/integrations/import", url.Values{"kind": {"git"}, "address": {"file://" + repo}, "ref": {"main"}})
	if !strings.HasPrefix(made.Header().Get("Location"), "/admin/integrations/reports") {
		t.Fatalf("answered %q", made.Header().Get("Location"))
	}
	page := admin.ask(t, "GET", "/admin/integrations/reports", nil).Body.String()
	if !strings.Contains(page, "git file://") || !strings.Contains(page, " @ ") {
		t.Error("the page does not say where the version came from and its commit")
	}
}

func TestAnAddressOnAPrivateNetworkIsRefusedOnThePage(t *testing.T) {
	here := everything(t)
	here.server.UseIntegrations(here.server.integrations, integration.Clone{})
	admin := here.as(t, boss)
	made := admin.ask(t, "POST", "/admin/integrations/import", url.Values{"kind": {"git"}, "address": {"https://169.254.169.254/latest/meta-data"}})
	if !strings.Contains(made.Header().Get("Location"), "refused=") {
		t.Errorf("answered %q", made.Header().Get("Location"))
	}
}

func TestSavingASetupsServersKeepsWhatElseItHolds(t *testing.T) {
	here, admin := withIntegrations(t)
	if err := here.server.setups.Save(setupWithStarters(), here.server.servers); err != nil {
		t.Fatal(err)
	}
	admin.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"starters"}, "server": {"memory"}})
	held, _ := here.server.setups.Get("starters")
	text, _ := json.Marshal(held)
	if len(held.Starters) != 1 || held.Wrote["x"] != "fp" {
		t.Errorf("saving the servers dropped what the setup held: %s", text)
	}
}

func setupWithStarters() setup.Setup {
	return setup.Setup{Name: "starters", Starters: []string{"Say hello"}, Wrote: map[string]string{"x": "fp"}}
}

func TestAnAdministratorAllowsTheProgramsOfOneVersionAndItIsRecorded(t *testing.T) {
	here, admin := withIntegrations(t)
	files := pluginFiles("1.0.0", "Summarise a report.")
	files["mcp.json"] = `{` + mcpHead + `, "mcpServers": {"db": {"type": "stdio", "command": "node", "args": ["${PLUGIN_ROOT}/db.js"]}}}`
	made := upload(t, admin, zipped(t, files))
	version := strings.SplitN(made.Header().Get("Location"), "version=", 2)[1]
	page := admin.ask(t, "GET", made.Header().Get("Location"), nil).Body.String()
	if !strings.Contains(page, "node ${PLUGIN_ROOT}/db.js") || !strings.Contains(page, "Allow the programs") {
		t.Errorf("the page does not show the program and the way to allow it:\n%s", page)
	}
	if refused := admin.ask(t, "POST", "/admin/integrations/reports/programs/allow", url.Values{"version": {version}}); !strings.Contains(refused.Header().Get("Location"), "refused=") {
		t.Errorf("the programs were allowed without the box being ticked: %q", refused.Header().Get("Location"))
	}
	if held, _ := here.server.integrations.Get("reports"); held.Programs != "" {
		t.Fatal("the programs were allowed without the box being ticked")
	}
	admin.ask(t, "POST", "/admin/integrations/reports/programs/allow", url.Values{"version": {version}, "understood": {"yes"}})
	held, _ := here.server.integrations.Get("reports")
	if held.Programs != version {
		t.Fatalf("programs = %q", held.Programs)
	}
	if page := admin.ask(t, "GET", made.Header().Get("Location"), nil).Body.String(); !strings.Contains(page, "Allowed.") || !strings.Contains(page, "Stop allowing them") {
		t.Error("the page does not say the programs are allowed")
	}
	admin.ask(t, "POST", "/admin/integrations/reports/programs/refuse", nil)
	events := ofKind(written(t, here.log), changelog.IntegrationPrograms)
	if len(events) != 2 || events[0].What["allowed"] != version || events[1].What["allowed"] != "" || events[1].What["was"] != version {
		t.Errorf("the change record has %+v", events)
	}
}

func TestAnAdministratorChoosesWhoAnIntegrationIsOnForAndAJobForSomebodyElseDoesNotStart(t *testing.T) {
	here, admin := withIntegrations(t)
	made := upload(t, admin, zipped(t, pluginFiles("1.0.0", "Summarise a report.")))
	version := strings.SplitN(made.Header().Get("Location"), "version=", 2)[1]
	admin.ask(t, "POST", "/admin/integrations/reports/enable", url.Values{"version": {version}})
	admin.ask(t, "POST", "/admin/integrations/reports/for", url.Values{"for": {"boss@example.com\n*@finance.example.com\n"}})
	held, _ := here.server.integrations.Get("reports")
	if strings.Join(held.For, ",") != "boss@example.com,*@finance.example.com" {
		t.Fatalf("for = %v", held.For)
	}
	events := ofKind(written(t, here.log), changelog.IntegrationScoped)
	if len(events) != 1 || events[0].What["now"] != "boss@example.com *@finance.example.com" {
		t.Errorf("the change record has %+v", events)
	}
	admin.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"team"}, "integration": {"reports"}})

	send := func(who *site) string {
		started := who.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}, "setup": {"team"}})
		where := started.Header().Get("Location")
		return who.ask(t, "POST", where+"/say", url.Values{"said": {"go"}}).Header().Get("Location")
	}
	if got := send(here.as(t, alice)); !strings.Contains(got, "refused=") {
		t.Errorf("a job for somebody the integration is not on for started: %q", got)
	}
	if got := send(admin); strings.Contains(got, "refused=") {
		t.Errorf("a job for somebody it is on for was refused: %q", got)
	}
}

func TestASkillSetIsDownloadedAsAPluginThatImportsAsAnIntegration(t *testing.T) {
	here, admin := withIntegrations(t)
	admin.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"reviewer"}})
	admin.ask(t, "POST", "/admin/skill-sets/reviewer/skills", url.Values{"skill": {"review"}, "description": {"Use to review."}, "body": {"Read the diff."}})
	got := admin.ask(t, "GET", "/admin/skill-sets/reviewer/plugin.zip", nil)
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("answered %d %q", got.Code, got.Header().Get("Content-Type"))
	}
	if forbidden := here.as(t, alice).ask(t, "GET", "/admin/skill-sets/reviewer/plugin.zip", nil); forbidden.Code != http.StatusForbidden {
		t.Errorf("a runner downloaded it: %d", forbidden.Code)
	}
	made := upload(t, admin, got.Body.Bytes())
	if !strings.HasPrefix(made.Header().Get("Location"), "/admin/integrations/reviewer") {
		t.Errorf("the download did not import: %q", made.Header().Get("Location"))
	}
	if held, err := here.server.integrations.Get("reviewer"); err != nil || len(held.Versions[0].Plugin.Skills) != 1 {
		t.Errorf("%+v %v", held, err)
	}
}
