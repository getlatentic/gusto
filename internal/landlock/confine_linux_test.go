package landlock

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The child half of TestATaskWithItsOwnTempSeesOnlyItsOwnFolders: behind the policy given in the environment, it tries each path
// and says which it could open.
func TestMain(m *testing.M) {
	if text := os.Getenv("GUSTO_LANDLOCK_CHILD"); text != "" {
		os.Exit(child(text))
	}
	os.Exit(m.Run())
}

func child(text string) int {
	policy, err := Load(text)
	if err != nil {
		return 3
	}
	if err := Apply(policy); err != nil {
		os.Stdout.WriteString("apply: " + err.Error())
		return 4
	}
	for _, path := range strings.Split(os.Getenv("GUSTO_LANDLOCK_TRY"), ",") {
		mode, name := path[:1], path[1:]
		var err error
		if mode == "r" {
			_, err = os.ReadFile(name)
		} else {
			err = os.WriteFile(name, []byte("x"), 0o600)
		}
		state := "ok"
		if err != nil {
			state = "refused"
			if !errors.Is(err, os.ErrPermission) {
				state = "error " + err.Error()
			}
		}
		os.Stdout.WriteString(path + "=" + state + "\n")
	}
	return 0
}

func TestATaskWithItsOwnTempSeesOnlyItsOwnFolders(t *testing.T) {
	if _, err := ABI(); err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	own, other, temp := filepath.Join(root, "own"), filepath.Join(root, "other"), filepath.Join(root, "temp")
	for _, dir := range []string{own, other, temp} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(own, "notes.md"), []byte("mine"), 0o600)
	os.WriteFile(filepath.Join(other, "secret.md"), []byte("not mine"), 0o600)
	shared, err := os.CreateTemp("/tmp", "gusto-shared-")
	if err != nil {
		t.Fatal(err)
	}
	shared.Close()
	defer os.Remove(shared.Name())
	policy, _ := Policy{Write: []string{own}, Temp: temp}.JSON()
	tries := []string{"r" + filepath.Join(own, "notes.md"), "w" + filepath.Join(own, "new.md"), "w" + filepath.Join(temp, "t"),
		"r" + filepath.Join(other, "secret.md"), "w" + filepath.Join(other, "planted.md"), "r" + shared.Name(), "w/tmp/gusto-planted"}
	self, _ := os.Executable()
	command := exec.Command(self, "-test.run=^$")
	command.Env = append(os.Environ(), "GUSTO_LANDLOCK_CHILD="+policy, "GUSTO_LANDLOCK_TRY="+strings.Join(tries, ","))
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := []string{"=ok", "=ok", "=ok", "=refused", "=refused", "=refused", "=refused"}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != len(tries) {
		t.Fatalf("the child said %q", out)
	}
	for i, line := range lines {
		if !strings.HasSuffix(line, want[i]) {
			t.Errorf("%s, want %s", line, want[i])
		}
	}
}
