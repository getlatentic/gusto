package setup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/gateway"
	"github.com/getlatentic/haas/internal/integration"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/schema"
)

const (
	pluginSchema = `"$schema": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"`
	mcpSchema    = `"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"`
)

func plugin(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	all := map[string]string{"plugin.json": `{` + pluginSchema + `, "name": "` + name + `", "version": "2.0.0"}`}
	for path, text := range files {
		all[path] = text
	}
	for path, text := range all {
		full := filepath.Join(dir, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func skill(name string) string {
	return "---\nname: " + name + "\ndescription: Use for " + name + ".\n---\n\nInstructions.\n"
}

const mixedServers = `{` + mcpSchema + `, "mcpServers": {
	"api": {"type": "streamable-http", "url": "https://api.example.com/mcp", "headers": {"X-Tenant": "acme"}},
	"local": {"type": "stdio", "command": "tool"},
	"old": {"type": "sse", "url": "https://old.example.com/sse"}}}`

// world is a store of setups beside a store of integrations, with the integration "reports" imported, and on when asked.
func world(t *testing.T, on bool) (*Store, *integration.Store, string) {
	t.Helper()
	integrations, err := integration.Open(filepath.Join(t.TempDir(), "integrations"))
	if err != nil {
		t.Fatal(err)
	}
	_, version, err := integrations.Import(plugin(t, "reports", map[string]string{
		"skills/summarize/SKILL.md":            skill("summarize"),
		"skills/summarize/scripts/run.sh":      "echo hi",
		"skills/summarize/references/guide.md": "guide",
		"mcp.json":                             mixedServers,
	}), integration.Source{Kind: "git", Address: "https://example.com/reports.git", Commit: "abc"}, "ada")
	if err != nil {
		t.Fatal(err)
	}
	if on {
		if _, err := integrations.Enable("reports", version.ID, "ada"); err != nil {
			t.Fatal(err)
		}
	}
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.UseIntegrations(integrations)
	return store, integrations, version.ID
}

func carry(t *testing.T, store *Store, name string) (*job.Setup, job.Dir, error) {
	t.Helper()
	dir := job.Dir(t.TempDir())
	carried, err := store.Carry(name, Servers{}, dir)
	return carried, dir, err
}

func TestAJobGetsTheWholeSkillAndTheServersItCanUseFromAnIntegrationThatIsOn(t *testing.T) {
	store, _, id := world(t, true)
	if err := store.Save(Setup{Name: "team", Integrations: []string{"reports"}}, Servers{}); err != nil {
		t.Fatal(err)
	}
	carried, dir, err := carry(t, store, "team")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(carried.Skills, ",") != "summarize" || strings.Join(carried.Gateway, ",") != "reports-api" || len(carried.Servers) != 0 {
		t.Fatalf("carried %+v", carried)
	}
	for _, file := range []string{"SKILL.md", "scripts/run.sh", "references/guide.md"} {
		if _, err := os.Stat(filepath.Join(dir.SetupDir(), "skills", "summarize", file)); err != nil {
			t.Errorf("the skill's %s did not arrive: %v", file, err)
		}
	}
	if use := carried.Integrations; len(use) != 1 || use[0].Name != "reports" || use[0].Version != id || use[0].Label != "2.0.0" || use[0].Commit != "abc" {
		t.Errorf("the record does not say what version the job had: %+v", use)
	}
	text, _ := os.ReadFile(filepath.Join(dir.SetupDir(), GatewayFile))
	var behind []gateway.Server
	if err := json.Unmarshal(text, &behind); err != nil || len(behind) != 1 || behind[0].URL != "https://api.example.com/mcp" || behind[0].Headers["X-Tenant"] != "acme" {
		t.Errorf("gateway.json = %s (%v)", text, err)
	}
	if info, err := os.Stat(filepath.Join(dir.SetupDir(), GatewayFile)); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the definitions are readable by more than their owner: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(dir.SetupDir(), "servers.json")); err == nil {
		t.Error("a server behind the gateway is also given to the harness directly")
	}
	record, _ := json.Marshal(carried)
	if strings.Contains(string(record), "api.example.com") || strings.Contains(string(record), "acme") {
		t.Errorf("a server's definition is in the evidence: %s", record)
	}
}

func TestAJobDoesNotStartWithAnIntegrationThatIsOffOrWasTurnedOffSinceTheSetupWasSaved(t *testing.T) {
	store, integrations, _ := world(t, false)
	if err := store.Save(Setup{Name: "team", Integrations: []string{"reports"}}, Servers{}); err != nil {
		t.Fatalf("a setup may name an integration that is off: %v", err)
	}
	if _, _, err := carry(t, store, "team"); err == nil || !strings.Contains(err.Error(), "which is off") {
		t.Fatalf("err = %v", err)
	}
	held, _ := store.Get("team")
	if len(held.Uses) != 1 || held.Uses[0].On || held.Uses[0].Skills != 0 {
		t.Errorf("uses = %+v", held.Uses)
	}
	all, _ := integrations.List()
	integrations.Enable("reports", all[0].Versions[0].ID, "ada")
	if _, _, err := carry(t, store, "team"); err != nil {
		t.Fatal(err)
	}
	integrations.Disable("reports", "ada")
	if _, _, err := carry(t, store, "team"); err == nil {
		t.Error("a job started with an integration that was turned off")
	}
}

func TestOnlyTheVersionThatIsOnIsGivenAndANewerOneWaits(t *testing.T) {
	store, integrations, _ := world(t, true)
	store.Save(Setup{Name: "team", Integrations: []string{"reports"}}, Servers{})
	integrations.Import(plugin(t, "reports", map[string]string{"skills/other/SKILL.md": skill("other")}), integration.Source{Kind: "upload"}, "ada")
	carried, _, err := carry(t, store, "team")
	if err != nil || strings.Join(carried.Skills, ",") != "summarize" {
		t.Fatalf("%+v %v", carried, err)
	}
}

func TestASetupNamesOnlyIntegrationsThatWereImportedOnceEach(t *testing.T) {
	store, _, _ := world(t, true)
	if err := store.Save(Setup{Name: "team", Integrations: []string{"nothing"}}, Servers{}); err == nil {
		t.Error("a setup named an integration nobody imported")
	}
	if err := store.Save(Setup{Name: "team", Integrations: []string{"reports", "reports"}}, Servers{}); err == nil {
		t.Error("a setup named an integration twice")
	}
	bare, _ := Open(t.TempDir())
	if err := bare.Save(Setup{Name: "team", Integrations: []string{"reports"}}, Servers{}); err == nil {
		t.Error("a deployment with no integrations kept a setup that names one")
	}
}

func TestTwoSkillsOfOneNameDoNotMeetInAJob(t *testing.T) {
	store, _, _ := world(t, true)
	store.Save(Setup{Name: "team", Integrations: []string{"reports"}}, Servers{})
	if err := store.Write("team", Skill{Name: "summarize", Description: "Our own.", Body: "ours"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := carry(t, store, "team"); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("err = %v", err)
	}
}

func TestWhatASetupNeedsOfAHarnessIncludesWhatItsIntegrationsGive(t *testing.T) {
	store, _, _ := world(t, true)
	store.Save(Setup{Name: "team", Integrations: []string{"reports"}}, Servers{})
	held, _ := store.Get("team")
	if held.Fits(false, true) == "" || held.Fits(true, false) == "" || held.Fits(true, true) != "" {
		t.Errorf("fits: %q %q %q", held.Fits(false, true), held.Fits(true, false), held.Fits(true, true))
	}
	if len(held.Uses) != 1 || held.Uses[0].Skills != 1 || held.Uses[0].Servers != 1 || len(held.Uses[0].Held) != 2 {
		t.Errorf("uses = %+v", held.Uses)
	}
	for _, why := range held.Uses[0].Held {
		if !strings.Contains(why, "not allowed") && !strings.Contains(why, "SSE") {
			t.Errorf("a held-back server does not say why: %q", why)
		}
	}
}

func TestTwoIntegrationsWithAServerOfTheSameNameStayApart(t *testing.T) {
	store, integrations, _ := world(t, true)
	_, other, _ := integrations.Import(plugin(t, "billing", map[string]string{"mcp.json": mixedServers}), integration.Source{Kind: "upload"}, "ada")
	integrations.Enable("billing", other.ID, "ada")
	store.Save(Setup{Name: "team", Integrations: []string{"reports", "billing"}}, Servers{})
	carried, _, err := carry(t, store, "team")
	if err != nil || strings.Join(carried.Gateway, ",") != "reports-api,billing-api" {
		t.Fatalf("%+v %v", carried, err)
	}
	if got := serverName("My.Plugin", "Some_Server"); got != "my-plugin-some-server" {
		t.Errorf("serverName = %q", got)
	}
}

func TestWhatAJobRecordsOfItsIntegrationsIsWhatTheJobSchemaAsks(t *testing.T) {
	store, _, _ := world(t, true)
	store.Save(Setup{Name: "team", Integrations: []string{"reports"}}, Servers{})
	carried, _, err := carry(t, store, "team")
	if err != nil {
		t.Fatal(err)
	}
	var whole struct {
		Properties struct {
			Setup json.RawMessage `json:"setup"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema.Job(), &whole); err != nil || len(whole.Properties.Setup) == 0 {
		t.Fatalf("the job schema has no setup: %v", err)
	}
	record, _ := json.Marshal(carried)
	if err := schema.Validate(whole.Properties.Setup, record); err != nil {
		t.Errorf("%v\n%s", err, record)
	}
	extra := strings.Replace(string(record), `"commit"`, `"unplanned":1,"commit"`, 1)
	if err := schema.Validate(whole.Properties.Setup, []byte(extra)); err == nil {
		t.Error("the schema takes a field it does not list, so the check says little")
	}
}

func TestTwoServersWhoseNamesBecomeTheSameNameStopTheJobAndNotOverwriteEachOther(t *testing.T) {
	store, integrations, _ := world(t, false)
	_, version, _ := integrations.Import(plugin(t, "clash", map[string]string{"mcp.json": `{` + mcpSchema + `, "mcpServers": {
		"a_b": {"type": "streamable-http", "url": "https://one.example.com/mcp"},
		"a-b": {"type": "streamable-http", "url": "https://two.example.com/mcp"}}}`}), integration.Source{Kind: "upload"}, "ada")
	integrations.Enable("clash", version.ID, "ada")
	store.Save(Setup{Name: "team", Integrations: []string{"clash"}}, Servers{})
	if _, _, err := carry(t, store, "team"); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("err = %v", err)
	}
}

func TestAProgramIsGivenToTheGatewayOnlyForTheVersionAnAdministratorAllowed(t *testing.T) {
	store, integrations, first := world(t, true)
	store.Save(Setup{Name: "team", Integrations: []string{"reports"}}, Servers{})
	_, dir, err := carry(t, store, "team")
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := os.ReadFile(filepath.Join(dir.SetupDir(), GatewayFile)); strings.Contains(string(text), "tool") {
		t.Fatalf("a program that nobody allowed was given to the gateway: %s", text)
	}

	if _, err := integrations.AllowPrograms("reports", first, "ada"); err != nil {
		t.Fatal(err)
	}
	held, _ := store.Get("team")
	if held.Uses[0].Servers != 2 || len(held.Uses[0].Held) != 1 {
		t.Errorf("uses after the programs were allowed: %+v", held.Uses[0])
	}
	carried, dir, err := carry(t, store, "team")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(carried.Gateway, ",") != "reports-api,reports-local" {
		t.Fatalf("gateway = %v", carried.Gateway)
	}
	var behind []gateway.Server
	text, _ := os.ReadFile(filepath.Join(dir.SetupDir(), GatewayFile))
	json.Unmarshal(text, &behind)
	var program gateway.Server
	for _, server := range behind {
		if server.IsProgram() {
			program = server
		}
	}
	active, _ := integrations.Active()
	if program.Command != "tool" || program.Plugin != active[0].Dir || program.Data != filepath.Join(dir.SetupDir(), "tooldata", "reports-local") {
		t.Errorf("the program as the gateway has it: %+v", program)
	}

	_, second, _ := integrations.Import(plugin(t, "reports", map[string]string{"mcp.json": `{` + mcpSchema + `, "mcpServers": {"local": {"type": "stdio", "command": "tool"}}}`}), integration.Source{Kind: "upload"}, "ada")
	integrations.Enable("reports", second.ID, "ada")
	carried, _, err = carry(t, store, "team")
	if err != nil || len(carried.Gateway) != 0 {
		t.Errorf("a new version's program ran on the old version's permission: %+v %v", carried, err)
	}
}

func TestAnIntegrationThatIsOnForSomePeopleIsCarriedForThemAndRefusesTheRest(t *testing.T) {
	store, integrations, _ := world(t, true)
	store.Save(Setup{Name: "team", Integrations: []string{"reports"}}, Servers{})
	integrations.SetFor("reports", []string{"ada@acme.example", "*@finance.acme.example"}, "ada")
	for asker, allowed := range map[string]bool{"ada@acme.example": true, "bo@finance.acme.example": true, "cy@acme.example": false, "": false} {
		dir := job.Dir(t.TempDir())
		carried, err := store.CarryFor("team", Servers{}, dir, asker)
		if allowed && (err != nil || len(carried.Skills) != 1) {
			t.Errorf("%q: %+v %v", asker, carried, err)
		}
		if !allowed && (err == nil || !strings.Contains(err.Error(), "some people and not for you")) {
			t.Errorf("%q was given what it is not on for: %v", asker, err)
		}
	}
	integrations.SetFor("reports", nil, "ada")
	if _, err := store.CarryFor("team", Servers{}, job.Dir(t.TempDir()), ""); err != nil {
		t.Errorf("an integration on for everybody was refused: %v", err)
	}
}

func TestASkillSetExportedAsAPluginIsAnIntegrationWithNoServers(t *testing.T) {
	store, _ := Open(t.TempDir())
	store.Save(Setup{Name: "reviewer", Servers: []string{"memory"}}, memory())
	store.Write("reviewer", Skill{Name: "review", Description: "Use when asked: \"review this\".\nSecond line.", Body: "Read the diff first."})
	store.Write("reviewer", Skill{Name: "summarise", Description: "Use to summarise.", Body: "Be brief."})
	data, err := store.Export("reviewer")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := integration.Unpack(data, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := integration.Read(dir)
	if err != nil || len(plugin.Problems) != 0 {
		t.Fatalf("%+v %v", plugin, err)
	}
	if plugin.Name != "reviewer" || len(plugin.Servers) != 0 || len(plugin.Skills) != 2 {
		t.Errorf("the exported plugin = %+v", plugin)
	}
	if plugin.Skills[0].Name != "review" || plugin.Skills[0].Description != `Use when asked: "review this". Second line.` {
		t.Errorf("a skill came back as %+v", plugin.Skills[0])
	}
	imported, err := integration.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, version, err := imported.Import(dir, integration.Source{Kind: "upload", Address: "reviewer.zip"}, "ada")
	if err != nil || len(version.Plugin.Skills) != 2 {
		t.Fatalf("%+v %v", version, err)
	}
	if _, err := store.Export("nothing"); err == nil {
		t.Error("a setup that is not there was exported")
	}
	store.Save(Setup{Name: "empty"}, memory())
	if _, err := store.Export("empty"); err == nil {
		t.Error("a setup with no skills was exported")
	}
}
