//go:build realharnesses

package runner

// This checks that a skill an integration brings, with the files beside it, is found and used by the harnesses it is placed for,
// through the same path a job takes: the plugin is imported, a setup carries it into a job, and the runner places it where the
// harness's adapter says it reads skills.
//
//	GUSTO_MEASURE=1 go test -tags realharnesses -run RealSkill -v ./internal/runner
//
// It makes short calls to the models each program is signed in to, so it only runs when asked for. A harness that is not signed in
// here is skipped, and says so.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/integration"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

const skillText = "---\nname: codeword\ndescription: Use when asked for the codeword, the secret word, or the passphrase of the project. Tells where the word is kept.\n---\n\nThe codeword is kept in references/word.md next to this file. Read that file and answer with the word in it.\n"

// carried is a job directory whose setup carries a plugin's skill, folder and all.
func carried(t *testing.T) (job.Dir, job.Spec) {
	t.Helper()
	source := t.TempDir()
	for name, text := range map[string]string{
		"plugin.json":                        `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"words"}`,
		"skills/codeword/SKILL.md":           skillText,
		"skills/codeword/references/word.md": "The codeword is tangerine-4711.\n",
		"skills/codeword/scripts/say.sh":     "#!/bin/sh\necho tangerine-4711\n",
	} {
		full := filepath.Join(source, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(text), 0o644)
	}
	store, err := integration.Open(filepath.Join(t.TempDir(), "integrations"))
	if err != nil {
		t.Fatal(err)
	}
	_, version, err := store.Import(source, integration.Source{Kind: "upload"}, "ada")
	if err != nil {
		t.Fatal(err)
	}
	store.Enable("words", version.ID, "ada")
	setups, _ := setup.Open(t.TempDir())
	setups.UseIntegrations(store)
	setups.Save(setup.Setup{Name: "words", Integrations: []string{"words"}}, setup.Servers{})
	dir := job.Dir(t.TempDir())
	dir.Make()
	setupRecord, err := setups.Carry("words", setup.Servers{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, job.Spec{Setup: setupRecord}
}

func ask(t *testing.T, workdir string, env []string, name string, args ...string) string {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = workdir
	command.Env = append(os.Environ(), env...)
	done := make(chan struct{})
	var out []byte
	go func() { out, _ = command.CombinedOutput(); close(done) }()
	select {
	case <-done:
	case <-time.After(180 * time.Second):
		command.Process.Kill()
		t.Fatalf("%s took too long", name)
	}
	return string(out)
}

func placed(t *testing.T, name, folder string) {
	t.Helper()
	adapter, err := harness.Load("", name)
	if err != nil {
		t.Fatal(err)
	}
	adapter.Skills = folder
	dir, spec := carried(t)
	if err := placeSkills(dir, spec, adapter); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"SKILL.md", "references/word.md", "scripts/say.sh"} {
		if _, err := os.Stat(filepath.Join(folder, "codeword", file)); err != nil {
			t.Fatalf("%s was not placed with the skill: %v", file, err)
		}
	}
}

func measuring(t *testing.T) {
	t.Helper()
	if os.Getenv("GUSTO_MEASURE") == "" {
		t.Skip("set GUSTO_MEASURE=1 to make short calls to the models these programs are signed in to")
	}
}

func TestRealSkillCodex(t *testing.T) {
	measuring(t)
	home := t.TempDir()
	placed(t, "codex", filepath.Join(home, "skills"))
	real, _ := os.UserHomeDir()
	os.Symlink(filepath.Join(real, ".codex", "auth.json"), filepath.Join(home, "auth.json"))
	out := ask(t, t.TempDir(), []string{"CODEX_HOME=" + home}, "codex", "exec", "--json", "--skip-git-repo-check", "--ephemeral", "-m", "gpt-5.5", "-s", "read-only", "What is the codeword? Answer with the word only.")
	if !strings.Contains(out, "tangerine-4711") {
		t.Errorf("codex did not find the skill and its reference:\n%s", out)
	}
}

func TestRealSkillOpenCode(t *testing.T) {
	measuring(t)
	config := t.TempDir()
	placed(t, "opencode", filepath.Join(config, "opencode", "skills"))
	out := ask(t, t.TempDir(), []string{"XDG_CONFIG_HOME=" + config}, "opencode", "run", "--format", "json", "-m", "opencode/big-pickle", "What is the codeword? Answer with the word only.")
	if !strings.Contains(out, `"tool":"skill"`) || !strings.Contains(out, "tangerine-4711") {
		t.Errorf("opencode did not find the skill and its reference:\n%s", out)
	}
}

// Claude Code is placed in $CLAUDE_CONFIG_DIR/skills, and its login belongs to that folder, so a different one is not signed in here.
// This places the skill where the same program reads a project's. It asks what skills Claude Code has, which says whether the skill
// was found, and then three times for the word, which says how often a small model uses it: that is the model's choice and not the
// placement's, so only the finding is required.
func TestRealSkillClaudeCodeProjectFolder(t *testing.T) {
	measuring(t)
	project := t.TempDir()
	placed(t, "claude", filepath.Join(project, ".claude", "skills"))
	listed := ask(t, project, nil, "claude", "-p", "List the names of the skills you have, separated by commas, and nothing else.", "--model", "haiku")
	if !strings.Contains(listed, "codeword") {
		t.Errorf("claude does not list the skill:\n%s", listed)
	}
	used := 0
	for range 3 {
		if strings.Contains(ask(t, project, nil, "claude", "-p", "What is the codeword? Use your skills. Answer with the word only.", "--model", "haiku"), "tangerine-4711") {
			used++
		}
	}
	t.Logf("claude found the skill's reference in %d of 3 calls", used)
}
