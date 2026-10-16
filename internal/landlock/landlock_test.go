package landlock

import (
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestAPolicyIsKeptInAFileAndTheCommandIsRunByGustoBehindIt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "landlock.json")
	want := Policy{Read: []string{"/home/ada/.nvm"}, Write: []string{"/work"}, ProxyPort: 7141}
	if err := want.Save(file); err != nil {
		t.Fatal(err)
	}
	got, err := Load(file)
	if err != nil || !slices.Equal(got.Read, want.Read) || !slices.Equal(got.Write, want.Write) || got.ProxyPort != 7141 {
		t.Fatalf("read back %+v, %v", got, err)
	}
	command := exec.Command("/usr/bin/engine", "--flag")
	Wrap("/h/gusto", file)(command)
	if command.Path != "/h/gusto" || !slices.Equal(command.Args, []string{"/h/gusto", "sandbox-run", file, "--", "/usr/bin/engine", "--flag"}) {
		t.Errorf("the command is %v %v", command.Path, command.Args)
	}
}
