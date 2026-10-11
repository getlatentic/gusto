package boot

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// credentialsWith runs the script that builds the proxy's credentials, with an aws that answers every parameter with a key.
func credentialsWith(t *testing.T, harnesses string) map[string]struct {
	Address   string   `json:"address"`
	Harnesses []string `json:"harnesses"`
	Wires     []string `json:"wires"`
} {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "aws"), []byte("#!/bin/sh\necho a-key\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../e2e/credentials.sh")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "creds")
	run := exec.Command("bash", script, out)
	run.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "BEDROCK_PARAMETER=/b", "DEEPSEEK_PARAMETER=/d", "DEEPSEEK_HARNESSES="+harnesses)
	if made, err := run.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, made)
	}
	held := map[string]struct {
		Address   string   `json:"address"`
		Harnesses []string `json:"harnesses"`
		Wires     []string `json:"wires"`
	}{}
	files, _ := filepath.Glob(filepath.Join(out, "*.json"))
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var one struct {
			Address   string   `json:"address"`
			Harnesses []string `json:"harnesses"`
			Wires     []string `json:"wires"`
		}
		if err := json.Unmarshal(raw, &one); err != nil {
			t.Fatal(err)
		}
		held[filepath.Base(file[:len(file)-len(".json")])] = one
	}
	return held
}

func TestDeepSeekCanBeLimitedTheAnthropicRouteIsClaudeCodesAndGemmaAnswersEveryOneTranslated(t *testing.T) {
	held := credentialsWith(t, "codex")
	if got := held["deepseek"].Harnesses; !slices.Equal(got, []string{"codex"}) {
		t.Errorf("the deepseek credential answers %v, not codex alone", got)
	}
	if got := held["anthropic"].Harnesses; !slices.Equal(got, []string{"claude"}) {
		t.Errorf("the anthropic credential answers %v, not claude alone", got)
	}
	if got := held["gemma"].Harnesses; got != nil {
		t.Errorf("the gemma credential is limited to %v", got)
	}
	if got := held["gemma"].Wires; !slices.Equal(got, []string{"chat"}) {
		t.Errorf("the gemma credential says its provider answers %v, so Claude Code's and Codex's calls are not translated", got)
	}
}

func TestWithNoHarnessesNamedTheDeepSeekCredentialAnswersEveryOne(t *testing.T) {
	if got := credentialsWith(t, "")["deepseek"].Harnesses; got != nil {
		t.Errorf("the deepseek credential is limited to %v when none were named", got)
	}
}

// demoDeepSeekHarnesses is the demo's own setting, through the line of demo.conf the template writes, read the way haas-boot reads it.
func demoDeepSeekHarnesses(t *testing.T) string {
	t.Helper()
	defaults, err := os.ReadFile("../../ansible/roles/demo/defaults/main.yml")
	if err != nil {
		t.Fatal(err)
	}
	setting := regexp.MustCompile(`(?m)^demo_deepseek_harnesses: (.*)$`).FindSubmatch(defaults)
	if setting == nil {
		t.Fatal("the demo's defaults name no demo_deepseek_harnesses")
	}
	template, err := os.ReadFile("../../ansible/roles/demo/templates/demo.conf.j2")
	if err != nil {
		t.Fatal(err)
	}
	line := regexp.MustCompile(`(?m)^DEEPSEEK_HARNESSES=.*$`).Find(template)
	conf := filepath.Join(t.TempDir(), "demo.conf")
	written := strings.ReplaceAll(string(line), "{{ demo_deepseek_harnesses }}", strings.TrimSpace(string(setting[1])))
	if err := os.WriteFile(conf, []byte(written+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	read, err := exec.Command("bash", "-c", `set -eu; . "$1"; printf %s "$DEEPSEEK_HARNESSES"`, "bash", conf).CombinedOutput()
	if err != nil {
		t.Fatalf("demo.conf cannot be read as haas-boot reads it: %v\n%s", err, read)
	}
	return string(read)
}

func TestTheDemoOffersEveryModelToEveryEngine(t *testing.T) {
	held := credentialsWith(t, demoDeepSeekHarnesses(t))
	for _, name := range []string{"deepseek", "gemma"} {
		if got := held[name].Harnesses; got != nil {
			t.Errorf("the %s credential is limited to %v", name, got)
		}
	}
}

func TestANewDemoChatStartsOnAPairTheDemoOffers(t *testing.T) {
	defaults, err := os.ReadFile("../../ansible/roles/demo/defaults/main.yml")
	if err != nil {
		t.Fatal(err)
	}
	pair := regexp.MustCompile(`(?m)^demo_chat_default: (\S+)=(\S+)$`).FindSubmatch(defaults)
	models := regexp.MustCompile(`(?m)^demo_models: (\S+)$`).FindSubmatch(defaults)
	if pair == nil || models == nil {
		t.Fatal("the demo's defaults name no demo_chat_default as engine=model, or no demo_models")
	}
	if !slices.Contains(strings.Split(string(models[1]), ","), string(pair[2])) {
		t.Errorf("a new chat starts on %s, which the demo does not offer: %s", pair[2], models[1])
	}
	env, err := os.ReadFile("../../ansible/roles/demo/templates/org.env.j2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "HAAS_CHAT_DEFAULT={{ demo_chat_default }}") {
		t.Error("an organisation's server is not given the demo's default for a new chat")
	}
}
