package hostboot

import (
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
"s3 cp") src=$3; dst=$4; f="$FAKE_STORE/${src#s3://*/}"; [ -f "$f" ] || { echo "no such object" >&2; exit 1; }; cp "$f" "$dst";;
"ssm get-parameter") echo '{"demo_name":"agentcloud.example.com","demo_zone":"example.com"}';;
esac`,
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
		"RELEASE=abc123", "BUCKET=store", "REGION=us-east-1", "LEASE_MINUTES=25", "MAX_HOURS=3",
		"HAAS_BUNDLE_DIR="+filepath.Join(dir, "bundle"), "HAAS_TMP="+filepath.Join(dir, "tmp"), "HAAS_BOOTSTRAP_LOG="+filepath.Join(dir, "boot.log"),
		"FAKE_STORE="+b.store, "FAKE_LOG="+b.log)
	os.MkdirAll(filepath.Join(dir, "tmp"), 0o755)
	return b
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
