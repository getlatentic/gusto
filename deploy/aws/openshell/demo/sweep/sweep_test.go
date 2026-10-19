package sweep

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const script = "../../demo-sweep.sh"

// sweep runs the script at 12:00 UTC on 2026-09-30, with the instances listed as "id launch-time" lines,
// and returns what it terminated and what it said.
func sweep(t *testing.T, instances string, extra ...string) (terminated []string, said string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0o755)
	log := filepath.Join(dir, "aws.log")
	fake := "#!/bin/bash\necho \"$*\" >> " + log + "\ncase \"$2\" in\ndescribe-instances) printf '%s\\n' \"$FAKE_INSTANCES\";;\nterminate-instances) ;;\nesac\n"
	os.WriteFile(filepath.Join(bin, "aws"), []byte(fake), 0o755)
	command := exec.Command("bash", script)
	command.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GUSTO_NOW=2026-09-30T12:00:00Z", "FAKE_INSTANCES="+instances)
	command.Env = append(command.Env, extra...)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	text, _ := os.ReadFile(log)
	for _, line := range strings.Split(string(text), "\n") {
		if strings.HasPrefix(line, "ec2 terminate-instances") {
			terminated = append(terminated, strings.Fields(line)[3])
		}
	}
	return terminated, string(out)
}

func TestOnlyAnInstanceUpLongerThanTheLimitIsTerminated(t *testing.T) {
	got, said := sweep(t, "i-old\t2026-09-30T04:30:00+00:00\ni-new\t2026-09-30T09:00:00+00:00\ni-edge\t2026-09-30T06:00:00+00:00")
	if len(got) != 1 || got[0] != "i-old" || !strings.Contains(said, "terminated i-old") {
		t.Errorf("terminated %v: %s", got, said)
	}
	got, _ = sweep(t, "i-old\t2026-09-30T04:30:00+00:00", "MAX_HOURS=9")
	if len(got) != 0 {
		t.Errorf("the limit is MAX_HOURS: %v", got)
	}
}

func TestWithNothingOldItSaysSoAndTerminatesNothing(t *testing.T) {
	got, said := sweep(t, "")
	if len(got) != 0 || !strings.Contains(said, "nothing tagged gusto-poc") {
		t.Errorf("%v: %s", got, said)
	}
}

func TestItAsksOnlyForInstancesTaggedForTheProject(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "aws.log")
	os.MkdirAll(filepath.Join(dir, "bin"), 0o755)
	os.WriteFile(filepath.Join(dir, "bin", "aws"), []byte("#!/bin/bash\necho \"$*\" >> "+log+"\n"), 0o755)
	command := exec.Command("bash", script)
	command.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	text, _ := os.ReadFile(log)
	if !strings.Contains(string(text), "Name=tag:project,Values=gusto-poc") || !strings.Contains(string(text), "pending,running") {
		t.Errorf("%s", text)
	}
}
