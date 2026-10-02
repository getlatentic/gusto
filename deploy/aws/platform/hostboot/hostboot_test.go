package hostboot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
"ssm get-parameter") echo '{"demo_name":"agentcloud.example.com","demo_zone":"example.com"}';;
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
		"RELEASE=abc123", "BUCKET=store", "REGION=us-east-1", "LEASE_MINUTES=25", "MAX_HOURS=3", "ROLE=all", "RUN=", "EFS=", "LIVE=off",
		"HAAS_BUNDLE_DIR="+filepath.Join(dir, "bundle"), "HAAS_TMP="+filepath.Join(dir, "tmp"), "HAAS_BOOTSTRAP_LOG="+filepath.Join(dir, "boot.log"),
		"HAAS_BAKED="+filepath.Join(dir, "not-baked"), "HAAS_SWAPFILE="+filepath.Join(dir, "swapfile"), "HAAS_FSTAB="+filepath.Join(dir, "fstab"), "HAAS_MEMINFO="+b.meminfo(8000),
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
	os.WriteFile(filepath.Join(src, "ansible", "demo.yml"), []byte("---\n"), 0o644)
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
	if deadman := index(lines, "systemd-run --on-active=3h --unit=haas-deadman"); deadman != 0 {
		t.Errorf("the host is scheduled to go before anything is fetched or run: %v", lines)
	}
	play := index(lines, "ansible-playbook ")
	if play < 0 {
		t.Fatalf("nothing was configured: %v", lines)
	}
	for _, want := range []string{"-c local", "-i spike,", "demo_bucket=store", "demo_region=us-east-1", "demo_image_key=imagekey0123456", "demo_lease_minutes=25", "demo_max_hours=3", "demo_check_public=false",
		"@" + filepath.Join(b.dir, "bundle", "demo-vars.json"), filepath.Join(b.dir, "bundle", "ansible", "site.yml") + " " + filepath.Join(b.dir, "bundle", "ansible", "demo.yml")} {
		if !strings.Contains(lines[play], want) {
			t.Errorf("the playbooks were run without %q:\n%s", want, lines[play])
		}
	}
	if index(lines, "systemd-run --on-active=15m") >= 0 {
		t.Errorf("a host that started is not scheduled to go for having failed: %v", lines)
	}
	if got, _ := os.ReadFile(filepath.Join(b.dir, "bundle", "demo-vars.json")); !strings.Contains(string(got), "agentcloud.example.com") {
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
	if index(lines, "systemd-run --on-active=15m --unit=haas-failed") < 0 {
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
	if err == nil || index(lines, "systemd-run --on-active=15m --unit=haas-failed") < 0 {
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

func TestAHostThatIsAllOfItHasOpenShellAndTheFrontDoorAndNoSharedFileSystem(t *testing.T) {
	play := playOf(t, "all")
	if !strings.Contains(play, "site.yml") || !strings.Contains(play, "demo_role=all") || strings.Contains(play, "demo_efs") || strings.Contains(play, "demo_run") {
		t.Errorf("%s", play)
	}
}

func TestAnOrchestratorIsConfiguredWithoutOpenShellAndWithTheSharedFileSystem(t *testing.T) {
	play := playOf(t, "orchestrator")
	for _, want := range []string{"demo_role=orchestrator", "demo_efs=fs-0123.efs.us-east-1.amazonaws.com", "demo_run=run-42", "demo.yml"} {
		if !strings.Contains(play, want) {
			t.Errorf("%q is not in %s", want, play)
		}
	}
	if strings.Contains(play, "site.yml") {
		t.Errorf("a front door that runs no computer has no use for OpenShell: %s", play)
	}
}

func TestACellIsConfiguredWithOpenShellAndTheSharedFileSystem(t *testing.T) {
	play := playOf(t, "cell")
	for _, want := range []string{"demo_role=cell", "demo_efs=fs-0123.efs.us-east-1.amazonaws.com", "demo_run=run-42", "site.yml", "demo.yml"} {
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
	lines, err := b.run("ROLE=orchestrator", "RUN=live", "EFS=fs-0123", "LIVE=on", "RELEASE=latest", "HAAS_LIVE_ENV="+liveEnv)
	if err != nil {
		t.Fatal(err)
	}
	if index(lines, "systemd-run") >= 0 {
		t.Errorf("a host meant to stay is scheduled to go: %v", lines)
	}
	play := index(lines, "ansible-playbook ")
	if play < 0 || !strings.Contains(lines[play], "demo_live=true") || !strings.Contains(lines[play], "demo_run=live") || !strings.Contains(lines[play], "demo_release=abc123") {
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
	lines, err := b.run("LIVE=on", "FAKE_ANSIBLE_FAILS=1", "HAAS_LIVE_ENV="+filepath.Join(b.dir, "live.env"))
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
	lines, err := b.run("HAAS_LIVE_ENV=" + liveEnv)
	if err != nil {
		t.Fatal(err)
	}
	if index(lines, "systemd-run --on-active=3h --unit=haas-deadman") != 0 {
		t.Errorf("%v", lines)
	}
	if _, err := os.Stat(liveEnv); err == nil {
		t.Error("a demo host kept an environment to be started again from")
	}
}

func TestASmallHostGetsASwapFileOnceAndALargeOneGetsNone(t *testing.T) {
	small := newBoot(t)
	small.release(false)
	small.env = append(small.env, "HAAS_MEMINFO="+small.meminfo(1024))
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

func TestAHostMadeFromTheImageOfACellDoesNotFetchPackagesAndAnOrdinaryHostDoes(t *testing.T) {
	ordinary := newBoot(t)
	ordinary.release(false)
	lines, err := ordinary.run()
	if err != nil {
		t.Fatal(err)
	}
	if index(lines, "apt-get ") < 0 {
		t.Errorf("an ordinary host installed no packages: %v", lines)
	}
	if play := index(lines, "ansible-playbook "); play < 0 || !strings.Contains(lines[play], "demo_bake=false") {
		t.Errorf("an ordinary host is told it is made to be imaged: %v", lines)
	}

	baked := newBoot(t)
	baked.release(false)
	marker := filepath.Join(baked.dir, "baked")
	os.WriteFile(marker, []byte("key\n"), 0o644)
	lines, err = baked.run("HAAS_BAKED=" + marker)
	if err != nil {
		t.Fatal(err)
	}
	if index(lines, "apt-get ") >= 0 {
		t.Errorf("a host from the image fetched packages: %v", lines)
	}
	if index(lines, "ansible-playbook ") < 0 {
		t.Errorf("a host from the image did not configure itself: %v", lines)
	}
}

func TestTheHostAnImageIsMadeFromPreparesTheRunnerImageAndIsNotStartedAsACell(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	done := filepath.Join(b.dir, "bake-done")
	lines, err := b.run("BAKE=on", "ROLE=cell", "HAAS_BAKE_DONE="+done)
	if err != nil {
		t.Fatal(err)
	}
	play := index(lines, "ansible-playbook ")
	if play < 0 || !strings.Contains(lines[play], "demo_bake=true") {
		t.Fatalf("the playbook was not told this host is made to be imaged: %v", lines)
	}
	survey := index(lines, "systemd-run --wait --collect --unit=haas-bake-survey")
	if survey < 0 || !strings.HasSuffix(lines[survey], "/usr/local/bin/haas survey") || survey < play {
		t.Errorf("the runner image was not prepared after the playbook: %v", lines)
	}
	if _, err := os.Stat(done); err != nil {
		t.Errorf("nothing says the host is ready to be imaged: %v", err)
	}
}

func TestAnOrdinaryHostPreparesNoImageAndLeavesNoMarker(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	done := filepath.Join(b.dir, "bake-done")
	lines, err := b.run("HAAS_BAKE_DONE=" + done)
	if err != nil {
		t.Fatal(err)
	}
	if index(lines, "systemd-run --wait --collect --unit=haas-bake-survey") >= 0 {
		t.Errorf("an ordinary host prepared an image: %v", lines)
	}
	if _, err := os.Stat(done); err == nil {
		t.Error("an ordinary host says it is ready to be imaged")
	}
}
