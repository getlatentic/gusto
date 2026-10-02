package boot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeOpenshell answers the three things the check asks of a sandbox. What a sandbox can open is OPEN, a list of host:port, and
// what its policy allows is POLICY_RULES, one name each. What a made-up name resolves to is ANSWER.
const fakeOpenshell = `#!/bin/bash
case "$2" in
list) [ -n "${NO_SANDBOX:-}" ] && { echo "NAME CREATED PHASE"; exit 0; }
      printf 'NAME CREATED PHASE\nc-test 2026-10-02 05:31:13 Ready\n';;
get) echo "  network_policies:"
     for r in ${POLICY_RULES:-haas_model_proxy}; do
       echo "    $r:"; echo "      name: $r"; echo "      endpoints:"
       echo "        - host: host.openshell.internal"; echo "          port: 9101"
     done
     echo "  other:";;
exec)
  shift 2
  while [ "$1" != -- ]; do shift; done; shift
  case "$1" in
  getent) echo "${ANSWER:-198.18.0.9} $3";;
  bash) shift 3; for t in "$@"; do case " $OPEN " in *" $t "*) echo "OPEN $t";; *) echo "closed $t";; esac; done;;
  esac;;
esac
`

func runCheck(t *testing.T, env ...string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openshell"), []byte(fakeOpenshell), 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", "../../e2e/isolation-on-host.sh")
	command.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PRIVATE_IP=10.0.0.5", "EFS_ADDRESS=10.0.0.9", "RESOLVER=10.0.0.2", "FRONT_DOOR=10.0.0.7")
	command.Env = append(command.Env, env...)
	out, err := command.CombinedOutput()
	return string(out), err
}

func TestTheIsolationCheckPassesWhenOnlyTheModelProxyIsOpenAndNamesAreAnsweredInTheSandbox(t *testing.T) {
	out, err := runCheck(t, "OPEN=host.openshell.internal:9101 127.0.0.53:53")
	if err != nil || !strings.Contains(out, "PASS: from inside a sandbox only the model proxy (host.openshell.internal:9101) is open") {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, tried := range []string{"169.254.169.254:80", "10.0.0.5:22", "10.0.0.5:8060", "10.0.0.9:2049", "10.0.0.2:53", "10.0.0.7:8050", "example.com:443", "1.1.1.1:443"} {
		if !strings.Contains(out, "closed "+tried) {
			t.Errorf("%s was not tried:\n%s", tried, out)
		}
	}
}

func TestTheIsolationCheckFailsAndNamesWhatASandboxCanOpen(t *testing.T) {
	for name, open := range map[string]string{
		"the metadata address":             "169.254.169.254:80",
		"the front door":                   "10.0.0.7:8050",
		"the file system":                  "10.0.0.9:2049",
		"the machine's agent":              "10.0.0.5:8060",
		"the internet":                     "1.1.1.1:443",
		"another port of the proxy's host": "host.openshell.internal:22",
	} {
		out, err := runCheck(t, "OPEN=host.openshell.internal:9101 "+open)
		if err == nil || !strings.Contains(out, "FAIL: a sandbox can open:") || !strings.Contains(out, "OPEN "+open) {
			t.Errorf("%s was not refused: %v\n%s", name, err, out)
		}
	}
}

func TestTheIsolationCheckFailsWhenTheProbeCannotTellBlockedFromBroken(t *testing.T) {
	out, err := runCheck(t, "OPEN=")
	if err == nil || !strings.Contains(out, "FAIL: the model proxy at host.openshell.internal:9101 is not open") {
		t.Errorf("%v\n%s", err, out)
	}
}

func TestTheIsolationCheckFailsWhenThePolicyAllowsMoreThanTheModelProxyOrNamesLeave(t *testing.T) {
	open := "OPEN=host.openshell.internal:9101"
	if out, err := runCheck(t, open, "POLICY_RULES=haas_model_proxy allowed_1"); err == nil || !strings.Contains(out, "allows more than the model proxy: allowed_1") {
		t.Errorf("an extra rule: %v\n%s", err, out)
	}
	if out, err := runCheck(t, open, "ANSWER=93.184.216.34"); err == nil || !strings.Contains(out, "may leave it") {
		t.Errorf("a real resolver: %v\n%s", err, out)
	}
}

func TestTheIsolationCheckSaysSoWhenNoSandboxIsRunning(t *testing.T) {
	out, err := runCheck(t, "NO_SANDBOX=1")
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 2 || !strings.Contains(out, "no sandbox is running") {
		t.Errorf("%v\n%s", err, out)
	}
}
