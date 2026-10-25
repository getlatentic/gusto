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

type heldCredential struct {
	Address   string   `json:"address"`
	Harnesses []string `json:"harnesses"`
	Wires     []string `json:"wires"`
}

// credentials runs the script that builds the proxy's credentials, with an aws that answers every parameter with a key.
func credentials(t *testing.T) map[string]heldCredential {
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
	run.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "BEDROCK_PARAMETER=/b", "DEEPSEEK_PARAMETER=/d")
	if made, err := run.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, made)
	}
	held := map[string]heldCredential{}
	files, _ := filepath.Glob(filepath.Join(out, "*.json"))
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var one heldCredential
		if err := json.Unmarshal(raw, &one); err != nil {
			t.Fatal(err)
		}
		held[strings.TrimSuffix(filepath.Base(file), ".json")] = one
	}
	return held
}

// The person chooses which engine runs on which model, so no credential names the engines it is for.
func TestTheDemoOffersEveryModelToEveryEngine(t *testing.T) {
	held := credentials(t)
	if len(held) != 4 {
		t.Fatalf("the script wrote %d credentials", len(held))
	}
	for name, credential := range held {
		if credential.Harnesses != nil {
			t.Errorf("the %s credential is limited to %v", name, credential.Harnesses)
		}
	}
	if got := held["gemma"].Wires; !slices.Equal(got, []string{"chat"}) {
		t.Errorf("the gemma credential says its provider answers %v, so Claude Code's and Codex's calls are not translated", got)
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
	if !strings.Contains(string(env), "GUSTO_CHAT_DEFAULT={{ demo_chat_default }}") {
		t.Error("an organization's server is not given the demo's default for a new chat")
	}
}

func TestTheCredentialsAreReadWithTheCommandAHostNames(t *testing.T) {
	dir := t.TempDir()
	reader := filepath.Join(dir, "read")
	if err := os.WriteFile(reader, []byte("#!/bin/sh\necho \"key-from-$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../e2e/credentials.sh")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "creds")
	run := exec.Command("bash", script, out)
	run.Env = append(os.Environ(), "PATH=/usr/bin:/bin", "READ_PARAMETER="+reader, "BEDROCK_PARAMETER=/b", "DEEPSEEK_PARAMETER=/d")
	if made, err := run.CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, made)
	}
	raw, err := os.ReadFile(filepath.Join(out, "deepseek.json"))
	if err != nil || !strings.Contains(string(raw), `"key-from-/d"`) {
		t.Errorf("the key was not read with the command named: %s %v", raw, err)
	}
}
