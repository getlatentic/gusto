package hostboot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
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
"ssm get-parameter") echo '{"demo_name":"gusto.example.com","demo_zone":"example.com"}';;
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
[ -z "${FAKE_ANSIBLE_FAILS:-}" ] || exit 2
for arg in "$@"; do case "$arg" in demo_root=*) root=${arg#demo_root=};; esac; done
if [ -n "${root:-}" ]; then mkdir -p "$root/usr/local/bin" "$root/etc/gusto"; echo program > "$root/usr/local/bin/gusto"; echo ROLE=cell > "$root/etc/gusto/demo.conf"; fi`,
	"mount": `#!/bin/bash
echo "mount $*" >> "$FAKE_LOG"
[ -z "${FAKE_MOUNT_FAILS:-}" ] || exit 32
if [ "$1" = --bind ]; then rmdir "$3" && ln -s "$2" "$3"; fi`,
	"systemctl": `#!/bin/bash
echo "systemctl $*" >> "$FAKE_LOG"`,
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
		"GUSTO_BUNDLE_DIR="+filepath.Join(dir, "bundle"), "GUSTO_TMP="+filepath.Join(dir, "tmp"), "GUSTO_BOOTSTRAP_LOG="+filepath.Join(dir, "boot.log"),
		"GUSTO_BAKED="+filepath.Join(dir, "not-baked"), "GUSTO_INSTANCE=i-0test", "GUSTO_WARMED="+dir, "GUSTO_WARM_WAIT=on", "GUSTO_SWAPFILE="+filepath.Join(dir, "swapfile"), "GUSTO_FSTAB="+filepath.Join(dir, "fstab"), "GUSTO_MEMINFO="+b.meminfo(8000),
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
	if deadman := index(lines, "systemd-run --on-active=3h --unit=gusto-deadman"); deadman != 0 {
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
	if got, _ := os.ReadFile(filepath.Join(b.dir, "bundle", "demo-vars.json")); !strings.Contains(string(got), "gusto.example.com") {
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
	lines, err := b.run("ROLE=orchestrator", "RUN=live", "EFS=fs-0123", "LIVE=on", "RELEASE=latest", "GUSTO_LIVE_ENV="+liveEnv)
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
		t.Error("a demo host kept an environment to be started again from")
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
	lines, err = baked.run("GUSTO_BAKED=" + marker)
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
	lines, err := b.run("BAKE=on", "ROLE=cell", "GUSTO_BAKE_DONE="+done)
	if err != nil {
		t.Fatal(err)
	}
	play := index(lines, "ansible-playbook ")
	if play < 0 || !strings.Contains(lines[play], "demo_bake=true") {
		t.Fatalf("the playbook was not told this host is made to be imaged: %v", lines)
	}
	survey := index(lines, "systemd-run --wait --collect --unit=gusto-bake-survey")
	if survey < 0 || !strings.HasSuffix(lines[survey], "/usr/local/bin/gusto scan") || survey < play {
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
	lines, err := b.run("GUSTO_BAKE_DONE=" + done)
	if err != nil {
		t.Fatal(err)
	}
	if index(lines, "systemd-run --wait --collect --unit=gusto-bake-survey") >= 0 {
		t.Errorf("an ordinary host prepared an image: %v", lines)
	}
	if _, err := os.Stat(done); err == nil {
		t.Error("an ordinary host says it is ready to be imaged")
	}
}

func TestAHostMadeFromTheImageOfACellGoesStraightToConfiguringItselfOnlyWhenTheReleaseIsForThatImage(t *testing.T) {
	for name, c := range map[string]struct {
		marker, bundleKey string
		fast              bool
	}{
		"the release is for the image":      {"abc123\n", "abc123\n", true},
		"the release expects another image": {"abc123\n", "def456\n", false},
		"the release carries no key":        {"abc123\n", "", false},
		"a host that is not from an image":  {"", "abc123\n", false},
	} {
		b := newBoot(t)
		b.release(false)
		release := filepath.Join(b.store, "releases", "abc123")
		src := filepath.Join(b.dir, "src")
		if c.bundleKey != "" {
			os.WriteFile(filepath.Join(src, "bake.key"), []byte(c.bundleKey), 0o644)
			if out, err := exec.Command("tar", "-czf", filepath.Join(release, "bundle.tgz"), "-C", src, ".").CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			sum, _ := exec.Command("shasum", "-a", "256", filepath.Join(release, "bundle.tgz")).Output()
			os.WriteFile(filepath.Join(release, "bundle.tgz.sha256"), []byte(strings.Fields(string(sum))[0]+"  bundle.tgz\n"), 0o644)
		}
		marker := filepath.Join(b.dir, "baked")
		if c.marker != "" {
			os.WriteFile(marker, []byte(c.marker), 0o644)
		}
		lines, err := b.run("GUSTO_BAKED="+marker, "ROLE=cell")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		play := index(lines, "ansible-playbook ")
		if play < 0 {
			t.Fatalf("%s: nothing was configured: %v", name, lines)
		}
		installs := strings.Contains(lines[play], "site.yml")
		if c.fast && (installs || !strings.Contains(lines[play], "demo_baked=true")) {
			t.Errorf("%s: the host installed what the image has:\n%s", name, lines[play])
		}
		if !c.fast && (!installs || !strings.Contains(lines[play], "demo_baked=false")) {
			t.Errorf("%s: the host did not install what it needs:\n%s", name, lines[play])
		}
	}
}

func TestAHostMadeFromTheImageOfACellWarmsItsDiskAndAnOrdinaryHostDoesNot(t *testing.T) {
	baked := newBoot(t)
	baked.release(false)
	marker := filepath.Join(baked.dir, "baked")
	os.WriteFile(marker, []byte("key\n"), 0o644)
	if _, err := baked.run("GUSTO_BAKED="+marker, "GUSTO_WARM_DIR="+baked.dir, "GUSTO_WARM_IMAGES="+baked.dir); err != nil {
		t.Fatal(err)
	}
	if log, _ := os.ReadFile(filepath.Join(baked.dir, "boot.log")); !strings.Contains(string(log), "warming the disk of the image") {
		t.Errorf("a host from the image did not warm its disk:\n%s", log)
	}
	ordinary := newBoot(t)
	ordinary.release(false)
	if _, err := ordinary.run(); err != nil {
		t.Fatal(err)
	}
	if log, _ := os.ReadFile(filepath.Join(ordinary.dir, "boot.log")); strings.Contains(string(log), "warming the disk") {
		t.Errorf("an ordinary host warmed a disk it does not have:\n%s", log)
	}
}

// kitted is a cell made from the image of key abc123, whose front door has put a kit on the shared file system.
func kitted(t *testing.T) (b *boot, sum string, extra []string) {
	t.Helper()
	b = newBoot(t)
	b.release(false)
	marker := filepath.Join(b.dir, "baked")
	os.WriteFile(marker, []byte("abc123\n"), 0o644)
	src := filepath.Join(b.dir, "kit-src")
	os.MkdirAll(filepath.Join(src, "usr", "local", "bin"), 0o755)
	os.WriteFile(filepath.Join(src, "usr", "local", "bin", "gusto"), []byte("program\n"), 0o755)
	efs := filepath.Join(b.dir, "efs")
	kits := filepath.Join(efs, "runs", "live", "kits")
	os.MkdirAll(kits, 0o755)
	kit := filepath.Join(b.dir, "kit.tgz")
	if out, err := exec.Command("tar", "-czf", kit, "-C", src, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	raw, _ := exec.Command("shasum", "-a", "256", kit).Output()
	sum = strings.Fields(string(raw))[0]
	os.Rename(kit, filepath.Join(kits, sum+".tgz"))
	os.MkdirAll(filepath.Join(b.dir, "root"), 0o755)
	extra = []string{"GUSTO_BAKED=" + marker, "ROLE=cell", "RUN=live", "EFS=fs-0123", "BAKE_KEY=abc123", "KIT=" + sum,
		"GUSTO_EFS_MOUNT=" + efs, "GUSTO_STATE=" + filepath.Join(b.dir, "state"), "GUSTO_ROOT=" + filepath.Join(b.dir, "root")}
	return b, sum, extra
}

func TestACellFromTheImageItsKitIsForUnpacksTheKitAndStartsWithNothingFetchedOrPlayed(t *testing.T) {
	b, _, extra := kitted(t)
	lines, err := b.run(extra...)
	if err != nil {
		t.Fatal(err)
	}
	for _, never := range []string{"aws ", "ansible-playbook ", "apt-get "} {
		if index(lines, never) >= 0 {
			t.Errorf("a cell with its kit ran %q: %v", never, lines)
		}
	}
	if index(lines, "mount -t nfs4") < 0 || index(lines, "mount --bind") < 0 {
		t.Errorf("the shared file system was not mounted: %v", lines)
	}
	if got, _ := os.ReadFile(filepath.Join(b.dir, "root", "usr", "local", "bin", "gusto")); string(got) != "program\n" {
		t.Errorf("the kit was not unpacked: %q", got)
	}
	enable, boot, start := index(lines, "systemctl enable gusto-lease.timer"), index(lines, "systemctl restart gusto-boot"), index(lines, "systemctl start gusto-lease.timer gusto-cell")
	if enable < 0 || boot < enable || start < boot || strings.Contains(strings.Join(lines, "\n"), "enable gusto-cell") {
		t.Errorf("the services are not turned on, the start-up work run, and the cell started, in that order: %v", lines)
	}
}

func TestACellWhoseKitIsNotTheOneItWasStartedWithIsConfiguredByThePlaybook(t *testing.T) {
	for name, change := range map[string]func(sum string) []string{
		"another sum":   func(string) []string { return []string{"KIT=" + strings.Repeat("0", 64)} },
		"another image": func(string) []string { return []string{"BAKE_KEY=def456"} },
		"no kit":        func(string) []string { return []string{"KIT=@KIT@"} },
	} {
		b, sum, extra := kitted(t)
		lines, err := b.run(append(extra, change(sum)...)...)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if index(lines, "ansible-playbook ") < 0 || index(lines, "systemctl ") >= 0 {
			t.Errorf("%s: the cell did not fall back to the playbook: %v", name, lines)
		}
		if _, err := os.Stat(filepath.Join(b.dir, "root", "usr", "local", "bin", "gusto")); err == nil {
			t.Errorf("%s: a kit that is not the one asked for was unpacked", name)
		}
	}
}

func TestAFrontDoorThatStartsCellsMakesTheirKitAndStartsThemWithItsSum(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	src := filepath.Join(b.dir, "src")
	os.WriteFile(filepath.Join(src, "bake.key"), []byte("abc123\n"), 0o644)
	release := filepath.Join(b.store, "releases", "abc123")
	if out, err := exec.Command("tar", "-czf", filepath.Join(release, "bundle.tgz"), "-C", src, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	raw, _ := exec.Command("shasum", "-a", "256", filepath.Join(release, "bundle.tgz")).Output()
	os.WriteFile(filepath.Join(release, "bundle.tgz.sha256"), []byte(strings.Fields(string(raw))[0]+"  bundle.tgz\n"), 0o644)
	kit := filepath.Join(b.dir, "cell-kit.tgz")
	lines, err := b.run("ROLE=orchestrator", "RUN=live", "EFS=fs-0123", "LIVE=on", "GUSTO_LIVE_ENV="+filepath.Join(b.dir, "live.env"), "GUSTO_KIT_FILE="+kit)
	if err != nil {
		t.Fatal(err)
	}
	made, played := index(lines, "ansible-playbook "), -1
	for i, line := range lines {
		if strings.HasPrefix(line, "ansible-playbook ") && strings.HasSuffix(line, "demo.yml") {
			played = i
		}
	}
	if made < 0 || !strings.HasSuffix(lines[made], "cell-kit.yml") || played < made {
		t.Fatalf("the kit is made before the front door is configured: %v", lines)
	}
	raw, _ = exec.Command("shasum", "-a", "256", kit).Output()
	sum := strings.Fields(string(raw))[0]
	for _, want := range []string{"demo_cell_kit=" + sum, "demo_cell_kit_file=" + kit, "demo_bake_key=abc123"} {
		if !strings.Contains(lines[played], want) {
			t.Errorf("%q is not in %s", want, lines[played])
		}
	}
	listed, _ := exec.Command("tar", "-tzf", kit).Output()
	if !strings.Contains(string(listed), "usr/local/bin/gusto") || !strings.Contains(string(listed), "etc/gusto/demo.conf") || strings.Contains(string(listed), "/\n") {
		t.Errorf("the kit holds the cell's files and no directories:\n%s", listed)
	}
}

func TestAHostMadeFromTheImageWarmsItsDiskOnceForEachInstance(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	marker := filepath.Join(b.dir, "baked")
	os.WriteFile(marker, []byte("abc123\n"), 0o644)
	warm := []string{"GUSTO_BAKED=" + marker, "GUSTO_WARM_DIR=" + b.dir, "GUSTO_WARM_IMAGES=" + b.dir}
	for _, c := range []struct {
		instance string
		warms    bool
	}{{"i-0first", true}, {"i-0first", false}, {"i-0second", true}} {
		os.Remove(filepath.Join(b.dir, "boot.log"))
		if _, err := b.run(append(warm, "GUSTO_INSTANCE="+c.instance)...); err != nil {
			t.Fatal(err)
		}
		log, _ := os.ReadFile(filepath.Join(b.dir, "boot.log"))
		if strings.Contains(string(log), "warming the disk") != c.warms {
			t.Errorf("%s warms its disk: %v, and should: %v", c.instance, !c.warms, c.warms)
		}
		if !c.warms {
			continue
		}
		recorded := false
		for deadline := time.Now().Add(time.Minute); !recorded && time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			_, err := os.Stat(filepath.Join(b.dir, ".warmed-"+c.instance))
			recorded = err == nil
		}
		if !recorded {
			t.Fatalf("%s did not record that its disk was read", c.instance)
		}
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
		"KIT": strings.Repeat("c", 64), "BAKE_KEY": strings.Repeat("d", 16),
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

// The disk is read in the background, after the script has gone on, and a host whose agents run in containers has none of what is
// read for virtual machines. Nothing of that reading may schedule the host to go: a cell that did powered off fifteen minutes after
// every start, with the organizations on it.
func TestReadingTheDiskInTheBackgroundNeverSchedulesTheHostToGo(t *testing.T) {
	b := newBoot(t)
	b.release(false)
	marker := filepath.Join(b.dir, "baked")
	os.WriteFile(marker, []byte("key\n"), 0o644)
	os.WriteFile(filepath.Join(b.dir, ".warmed-i-0test"), nil, 0o644)
	empty := t.TempDir()
	if _, err := b.run("GUSTO_BAKED="+marker, "GUSTO_WARM_WAIT=", "GUSTO_WARM_DIR="+empty, "GUSTO_WARM_IMAGES="+empty); err != nil {
		t.Fatal(err)
	}
	// What is read is nothing, so the background is done within milliseconds; the wait is generous.
	time.Sleep(2 * time.Second)
	if text, _ := os.ReadFile(b.log); strings.Contains(string(text), "gusto-failed") {
		t.Errorf("a host that started is scheduled to go:\n%s", text)
	}
}
