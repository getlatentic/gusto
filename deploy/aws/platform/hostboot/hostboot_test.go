package hostboot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const script = "../user-data.sh"

// Fakes of what the script calls. What is done is appended to a log, in order.
var fakes = map[string]string{
	"systemd-run": `#!/bin/bash
echo "systemd-run $*" >> "$FAKE_LOG"`,
	"apt-get": `#!/bin/bash
echo "apt-get $*" >> "$FAKE_LOG"`,
	"aws": `#!/bin/bash
echo "aws $1 $2 ${3:-}" >> "$FAKE_LOG"
case "$1 $2" in
"s3 cp") src=$3; dst=$4; f="$FAKE_STORE/${src#s3://*/}"; [ -f "$f" ] || { echo "no such object" >&2; exit 1; }; if [ "$dst" = - ]; then cat "$f"; else cp "$f" "$dst"; fi;;
"ssm get-parameter") echo '{"cloud_domain":"gusto.example.com","cloud_zone":"example.com"}';;
esac`,
	"fallocate": `#!/bin/bash
echo "fallocate $*" >> "$FAKE_LOG"; : > "$3"`,
	"mkswap": `#!/bin/bash
echo "mkswap $*" >> "$FAKE_LOG"`,
	"swapon": `#!/bin/bash
echo "swapon $*" >> "$FAKE_LOG"`,
	"sha256sum": `#!/bin/bash
exec shasum -a 256 "$@"`,
	"ansible-playbook": `#!/bin/bash
echo "ansible-playbook $*" >> "$FAKE_LOG"
[ -z "${FAKE_ANSIBLE_FAILS:-}" ] || exit 2`,
}

type boot struct {
	t          *testing.T
	dir, store string
	log        string
	env        []string
}

func newBoot(t *testing.T) *boot {
	t.Helper()
	dir := t.TempDir()
	b := &boot{t: t, dir: dir, store: filepath.Join(dir, "store"), log: filepath.Join(dir, "log")}
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0o755)
	os.MkdirAll(filepath.Join(b.store, "releases", "abc123"), 0o755)
	for name, text := range fakes {
		os.WriteFile(filepath.Join(bin, name), []byte(text+"\n"), 0o755)
	}
	b.env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RELEASE=abc123", "BUCKET=store", "REGION=us-east-1", "LEASE_MINUTES=25", "MAX_HOURS=3", "ROLE=all", "RUN=", "EFS=", "LIVE=off", "GUSTO_MACHINE=x86_64",
		"GUSTO_BUNDLE_DIR="+filepath.Join(dir, "bundle"), "GUSTO_TMP="+filepath.Join(dir, "tmp"), "GUSTO_BOOTSTRAP_LOG="+filepath.Join(dir, "boot.log"),
		"GUSTO_SWAPFILE="+filepath.Join(dir, "swapfile"), "GUSTO_FSTAB="+filepath.Join(dir, "fstab"), "GUSTO_MEMINFO="+b.meminfo(8000),
		"FAKE_STORE="+b.store, "FAKE_LOG="+b.log)
	os.MkdirAll(filepath.Join(dir, "tmp"), 0o755)
	return b
}

// meminfo is a file that says how much memory the host has, in megabytes.
func (b *boot) meminfo(megabytes int) string {
	path := filepath.Join(b.dir, "meminfo")
	os.WriteFile(path, []byte(fmt.Sprintf("MemTotal:       %d kB\n", megabytes*1024)), 0o644)
	return path
}

// release puts a bundle of two playbooks and an image key in the store, with the checksum CI would write.
func (b *boot) release(corrupt bool) {
	b.t.Helper()
	src := filepath.Join(b.dir, "src")
	os.MkdirAll(filepath.Join(src, "ansible"), 0o755)
	os.WriteFile(filepath.Join(src, "ansible", "site.yml"), []byte("---\n"), 0o644)
	os.WriteFile(filepath.Join(src, "ansible", "cloud.yml"), []byte("---\n"), 0o644)
	os.WriteFile(filepath.Join(src, "image.key"), []byte("imagekey0123456\n"), 0o644)
	release := filepath.Join(b.store, "releases", "abc123")
	if out, err := exec.Command("tar", "-czf", filepath.Join(release, "bundle.tgz"), "-C", src, ".").CombinedOutput(); err != nil {
		b.t.Fatalf("%v: %s", err, out)
	}
	sum, err := exec.Command("shasum", "-a", "256", filepath.Join(release, "bundle.tgz")).Output()
	if err != nil {
		b.t.Fatal(err)
	}
	line := strings.Fields(string(sum))[0] + "  bundle.tgz\n"
	if corrupt {
		line = strings.Repeat("0", 64) + "  bundle.tgz\n"
	}
	os.WriteFile(filepath.Join(release, "bundle.tgz.sha256"), []byte(line), 0o644)
}

func (b *boot) run(extra ...string) (lines []string, err error) {
	b.t.Helper()
	command := exec.Command("bash", script)
	command.Env = append(b.env, extra...)
	out, err := command.CombinedOutput()
	if err != nil {
		b.t.Logf("%s", out)
	}
	text, _ := os.ReadFile(b.log)
	for _, line := range strings.Split(strings.TrimSpace(string(text)), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, err
}

func index(lines []string, prefix string) int {
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return i
		}
	}
	return -1
}

func TestAHostIsMadeFromTheReleaseWithItsLeaseAndADeadmanScheduledBeforeAnythingElse(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	lines, err := b.run()
	if err != nil {
		t.Fatal(err)
	}
	if deadman := index(lines, "systemd-run --on-active=3h --unit=gusto-deadman"); deadman != 0 {
		t.Errorf("the host is scheduled to go before anything is fetched or run: %v", lines)
	}
	play := index(lines, "ansible-playbook ")
	if play < 0 {
		t.Fatalf("nothing was configured: %v", lines)
	}
	for _, want := range []string{"-c local", "-i host,", "cloud_bucket=store", "cloud_region=us-east-1", "cloud_image_key=imagekey0123456", "cloud_lease_minutes=25", "cloud_max_hours=3", "cloud_check_public=false",
		"@" + filepath.Join(b.dir, "bundle", "cloud-vars.json"), filepath.Join(b.dir, "bundle", "ansible", "site.yml") + " " + filepath.Join(b.dir, "bundle", "ansible", "cloud.yml")} {
		if !strings.Contains(lines[play], want) {
			t.Errorf("the playbooks were run without %q:\n%s", want, lines[play])
		}
	}
	if index(lines, "systemd-run --on-active=15m") >= 0 {
		t.Errorf("a host that started is not scheduled to go for having failed: %v", lines)
	}
	if got, _ := os.ReadFile(filepath.Join(b.dir, "bundle", "cloud-vars.json")); !strings.Contains(string(got), "gusto.example.com") {
		t.Errorf("the host's settings were read from Parameter Store: %q", got)
	}
}

func TestABundleThatIsNotTheOneCIMadeIsNeverRunAndTheHostGoes(t *testing.T) {
	b := newBoot(t)
	b.release(true)
	lines, err := b.run()
	if err == nil {
		t.Fatal("a bundle that fails its checksum started a host")
	}
	if index(lines, "ansible-playbook ") >= 0 {
		t.Errorf("something of an unchecked bundle ran: %v", lines)
	}
	if _, err := os.Stat(filepath.Join(b.dir, "bundle", "ansible")); err == nil {
		t.Error("an unchecked bundle was unpacked")
	}
	if index(lines, "systemd-run --on-active=15m --unit=gusto-failed") < 0 {
		t.Errorf("a host that failed to start is removed: %v", lines)
	}
}

func TestAHostWithNoBundleOrWhoseConfigurationFailsGoesInFifteenMinutes(t *testing.T) {
	missing := newBoot(t)
	lines, err := missing.run()
	if err == nil || index(lines, "systemd-run --on-active=15m") < 0 || index(lines, "ansible-playbook ") >= 0 {
		t.Errorf("no bundle: %v, %v", err, lines)
	}
	failing := newBoot(t)
	failing.release(false)
	lines, err = failing.run("FAKE_ANSIBLE_FAILS=1")
	if err == nil || index(lines, "systemd-run --on-active=15m --unit=gusto-failed") < 0 {
		t.Errorf("a configuration that failed: %v, %v", err, lines)
	}
}

func playOf(t *testing.T, role string) string {
	t.Helper()
	b := newBoot(t)
	b.release(false)
	lines, err := b.run("ROLE="+role, "RUN=run-42", "EFS=fs-0123")
	if err != nil {
		t.Fatal(err)
	}
	play := index(lines, "ansible-playbook ")
	if play < 0 {
		t.Fatalf("nothing was configured: %v", lines)
	}
	return lines[play]
}

func TestAHostThatIsAllOfItHasOpenShellAndGustoCloudAndNoSharedFileSystem(t *testing.T) {
	play := playOf(t, "all")
	if !strings.Contains(play, "site.yml") || !strings.Contains(play, "cloud_role=all") || strings.Contains(play, "cloud_efs") || strings.Contains(play, "cloud_run") {
		t.Errorf("%s", play)
	}
}

func TestAnOrchestratorIsConfiguredWithoutOpenShellAndWithTheSharedFileSystem(t *testing.T) {
	play := playOf(t, "orchestrator")
	for _, want := range []string{"cloud_role=orchestrator", "cloud_efs=fs-0123.efs.us-east-1.amazonaws.com", "cloud_run=run-42", "cloud.yml"} {
		if !strings.Contains(play, want) {
			t.Errorf("%q is not in %s", want, play)
		}
	}
	if strings.Contains(play, "site.yml") {
		t.Errorf("a gusto cloud server that runs no computer has no use for OpenShell: %s", play)
	}
}

func TestANodeIsConfiguredWithOpenShellAndTheSharedFileSystem(t *testing.T) {
	play := playOf(t, "cell")
	for _, want := range []string{"cloud_role=cell", "cloud_efs=fs-0123.efs.us-east-1.amazonaws.com", "cloud_run=run-42", "site.yml", "cloud.yml"} {
		if !strings.Contains(play, want) {
			t.Errorf("%q is not in %s", want, play)
		}
	}
}

func TestAHostThatStaysUpHasNoDeadmanNoFailureTimerAndKeepsWhatItWasStartedWith(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	os.WriteFile(filepath.Join(b.store, "releases", "latest"), []byte("abc123"), 0o644)
	liveEnv := filepath.Join(b.dir, "etc", "live.env")
	lines, err := b.run("ROLE=orchestrator", "RUN=live", "EFS=fs-0123", "LIVE=on", "RELEASE=latest", "GUSTO_LIVE_ENV="+liveEnv)
	if err != nil {
		t.Fatal(err)
	}
	if index(lines, "systemd-run") >= 0 {
		t.Errorf("a host meant to stay is scheduled to go: %v", lines)
	}
	play := index(lines, "ansible-playbook ")
	if play < 0 || !strings.Contains(lines[play], "cloud_live=true") || !strings.Contains(lines[play], "cloud_run=live") || index(lines, "aws s3 cp s3://store/releases/abc123/bundle.tgz") < 0 {
		t.Fatalf("the newest release is resolved and the host is told it is live: %v", lines)
	}
	text, _ := os.ReadFile(liveEnv)
	for _, want := range []string{"ROLE=orchestrator", "RUN=live", "EFS=fs-0123", "LIVE=on", "BUCKET=store"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("%q is not kept in %s", want, text)
		}
	}
}

func TestAHostThatStaysUpIsNotPoweredOffWhenItsConfigurationFails(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	lines, err := b.run("LIVE=on", "FAKE_ANSIBLE_FAILS=1", "GUSTO_LIVE_ENV="+filepath.Join(b.dir, "live.env"))
	if err == nil {
		t.Fatal("a failed configuration succeeded")
	}
	if index(lines, "systemd-run") >= 0 {
		t.Errorf("a live host that fails to configure is left for a person to look at: %v", lines)
	}
}

func TestAHostThatIsNotLiveIsStillScheduledToGoAndDoesNotKeepAnEnvironment(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	liveEnv := filepath.Join(b.dir, "live.env")
	lines, err := b.run("GUSTO_LIVE_ENV=" + liveEnv)
	if err != nil {
		t.Fatal(err)
	}
	if index(lines, "systemd-run --on-active=3h --unit=gusto-deadman") != 0 {
		t.Errorf("%v", lines)
	}
	if _, err := os.Stat(liveEnv); err == nil {
		t.Error("a host kept an environment to be started again from")
	}
}

func TestASmallHostGetsASwapFileOnceAndALargeOneGetsNone(t *testing.T) {
	small := newBoot(t)
	small.release(false)
	small.env = append(small.env, "GUSTO_MEMINFO="+small.meminfo(1024))
	lines, err := small.run()
	if err != nil {
		t.Fatal(err)
	}
	if index(lines, "fallocate -l 1G ") < 0 || index(lines, "mkswap ") < 0 || index(lines, "swapon ") < 0 {
		t.Errorf("a host with a gigabyte has no swap: %v", lines)
	}
	if fstab, _ := os.ReadFile(filepath.Join(small.dir, "fstab")); strings.Count(string(fstab), "swapfile") != 1 {
		t.Errorf("the swap file is not in fstab once: %q", fstab)
	}
	again, err := small.run()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.Join(again, "\n"), "swapon "); n != 1 {
		t.Errorf("an upgrade made swap again: %d\n%v", n, again)
	}

	large := newBoot(t)
	large.release(false)
	lines, err = large.run()
	if err != nil {
		t.Fatal(err)
	}
	if index(lines, "fallocate ") >= 0 {
		t.Errorf("a host with 8 gigabytes got swap: %v", lines)
	}
}

// EC2 refuses user-data over 16,384 bytes, before it is encoded, and a host that is launched with more is not launched at all. The
// script is filled in with the longest each setting can be, and must leave room under the limit for what is added to it.
func TestTheStartUpScriptWithItsSettingsFilledInLeavesRoomUnderWhatEC2Takes(t *testing.T) {
	const limit, room = 16384, 2048
	text, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	longest := map[string]string{
		"RELEASE": strings.Repeat("a", 40), "BUCKET": strings.Repeat("b", 63), "REGION": "ap-southeast-2", "LEASE_MINUTES": "1440",
		"MAX_HOURS": "168", "ROLE": "orchestrator", "RUN": strings.Repeat("r", 63), "EFS": "fs-" + strings.Repeat("0", 17), "LIVE": "off",
	}
	filled := string(text)
	for name, value := range longest {
		filled = strings.ReplaceAll(filled, "@"+name+"@", value)
	}
	if blank := regexp.MustCompile(`@[A-Z_]+@`).FindString(filled); blank != "" {
		t.Fatalf("the script has %s, which this test does not know the longest of", blank)
	}
	if len(filled) > limit-room {
		t.Errorf("the script is %d bytes filled in, and EC2's limit is %d with %d left for what is added: move something into the bundle", len(filled), limit, room)
	}
}

func TestAnArmHostRunsTheBundlesArmProgramsAndTheRunnerImageBuiltForArm(t *testing.T) {
	for _, c := range []struct{ machine, key, programs string }{
		{"x86_64", "cloud_image_key=imagekey0123456 ", ".state"},
		{"aarch64", "cloud_image_key=imagekey0123456-arm64 ", ".state/arm64"},
	} {
		b := newBoot(t)
		b.release(false)
		lines, err := b.run("GUSTO_MACHINE=" + c.machine)
		if err != nil {
			t.Fatal(err)
		}
		play := lines[index(lines, "ansible-playbook ")]
		if !strings.Contains(play, c.key) || !strings.Contains(play, "cloud_programs="+filepath.Join(b.dir, "bundle", c.programs)+" ") {
			t.Errorf("a host of %s: %s", c.machine, play)
		}
	}
}

func TestAHostThatRunsEveryServerInOneProcessWithAgentsInContainersHasNoOpenShellAndKeepsThatForUpgrades(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	liveEnv := filepath.Join(b.dir, "etc", "live.env")
	lines, err := b.run("LIVE=on", "SERVERS=shared", "SANDBOX=container", "COMPUTER_MEMORY=1Gi", "GUSTO_LIVE_ENV="+liveEnv)
	if err != nil {
		t.Fatal(err)
	}
	play := lines[index(lines, "ansible-playbook ")]
	for _, want := range []string{"cloud_servers=shared", "public_sandbox=container", "cloud_demo_sandbox=container", "cloud_computer_memory=1Gi", "cloud_role=all"} {
		if !strings.Contains(play, want) {
			t.Errorf("%q is not in %s", want, play)
		}
	}
	if strings.Contains(play, "site.yml") {
		t.Errorf("agents in containers need no OpenShell: %s", play)
	}
	text, _ := os.ReadFile(liveEnv)
	for _, want := range []string{"SERVERS=shared\n", "SANDBOX=container\n", "COMPUTER_MEMORY=1Gi\n"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("%q is not kept in %s", want, text)
		}
	}
}

func TestAHostToldNothingOfServersOrSandboxesRunsAProcessForEachAndChoosesNoSandbox(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	lines, err := b.run()
	if err != nil {
		t.Fatal(err)
	}
	play := lines[index(lines, "ansible-playbook ")]
	if !strings.Contains(play, "cloud_servers=on") || strings.Contains(play, "_sandbox=") || strings.Contains(play, "cloud_computer_memory") {
		t.Errorf("%s", play)
	}
}
