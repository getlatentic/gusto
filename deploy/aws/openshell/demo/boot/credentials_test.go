package boot

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// credentialsWith runs the script that builds the proxy's credentials, with an aws that answers every parameter with a key.
func credentialsWith(t *testing.T, harnesses string) map[string]struct {
	Address   string   `json:"address"`
	Harnesses []string `json:"harnesses"`
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
		}
		if err := json.Unmarshal(raw, &one); err != nil {
			t.Fatal(err)
		}
		held[filepath.Base(file[:len(file)-len(".json")])] = one
	}
	return held
}

func TestTheDemoGivesDeepSeekToCodexAndTheAnthropicRouteToClaudeCodeAndGemmaToTheRest(t *testing.T) {
	held := credentialsWith(t, "codex")
	if got := held["deepseek"].Harnesses; !slices.Equal(got, []string{"codex"}) {
		t.Errorf("the deepseek credential answers %v, not codex alone", got)
	}
	if got := held["anthropic"].Harnesses; !slices.Equal(got, []string{"claude"}) {
		t.Errorf("the anthropic credential answers %v, not claude alone", got)
	}
	if got := held["gemma"].Harnesses; !slices.Equal(got, []string{"opencode", "pi", "hermes"}) {
		t.Errorf("the gemma credential answers %v", got)
	}
}

func TestWithNoHarnessesNamedTheDeepSeekCredentialAnswersEveryOne(t *testing.T) {
	if got := credentialsWith(t, "")["deepseek"].Harnesses; got != nil {
		t.Errorf("the deepseek credential is limited to %v when none were named", got)
	}
}
