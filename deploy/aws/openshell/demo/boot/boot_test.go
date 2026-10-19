package boot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const script = "../../ansible/roles/demo/files/gusto-boot"

// Fakes of the commands the script calls. The store is a directory, and everything done is
// appended to a log, so a test reads what the host did and in what order.
var fakes = map[string]string{
	"aws": `#!/bin/bash
echo "aws $*" >> "$FAKE_LOG"
case "$1 $2" in
"ssm get-parameter")
  while [ $# -gt 0 ]; do [ "$1" = --name ] && name=$2; shift; done
  case " $FAKE_MISSING " in *" $name "*) echo "ParameterNotFound: $name" >&2; exit 254;; esac
  echo "value-of-$name";;
"s3 cp")
  src=$3; dst=$4
  case "$src" in s3://*) f="$FAKE_STORE/${src#s3://*/}"; [ -f "$f" ] || exit 1; cp "$f" "$dst";;
    *) f="$FAKE_STORE/${dst#s3://*/}"; mkdir -p "$(dirname "$f")"; cp "$src" "$f";; esac;;
*) echo "unexpected: $*" >&2; exit 2;;
esac`,
	"openssl": `#!/bin/bash
f=${@: -1}
[ -f "$f" ] && grep -q "^VALID" "$f"`,
	"certbot": `#!/bin/bash
echo "certbot $*" >> "$FAKE_LOG"
while [ $# -gt 0 ]; do [ "$1" = --cert-name ] && name=$2; shift; done
mkdir -p "$GUSTO_LE/live/$name"; echo "VALID issued" > "$GUSTO_LE/live/$name/fullchain.pem"`,
	"gusto-dns": `#!/bin/bash
echo "gusto-dns $*" >> "$FAKE_LOG"`,
	"curl": `#!/bin/bash
case "$*" in *-X\ PUT*) echo token;; *public-ipv4*) echo 203.0.113.7;; *local-ipv4*) echo 10.0.7.21;; *instance-id*) echo i-0abc123def4567890;; *) exit 22;; esac`,
	"credentials.sh": `#!/bin/bash
echo "credentials $BEDROCK_PARAMETER $DEEPSEEK_PARAMETER $1" >> "$FAKE_LOG"
mkdir -p "$1"; echo '{}' > "$1/platform.json"`,
}

type host struct {
	t                     *testing.T
	dir, store, log, conf string
	env                   []string
}

func newHost(t *testing.T) *host {
	t.Helper()
	dir := t.TempDir()
	h := &host{t: t, dir: dir, store: filepath.Join(dir, "store"), log: filepath.Join(dir, "log"), conf: filepath.Join(dir, "demo.conf")}
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0o755)
	os.MkdirAll(h.store, 0o755)
	for name, text := range fakes {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(text+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(h.conf, []byte(strings.Join([]string{
		"AWS_REGION=us-east-1", "NAME=gusto.example.com", "ZONE=example.com", "BUCKET=store", "CERT_EMAIL=ops@example.com", "IMAGE_KEY=key1",
		"DNS_TOKEN_PARAMETER=/gusto/poc/cloudflare-dns-token", "MAIL_TOKEN_PARAMETER=/gusto/poc/cloudflare-mail-token",
		"MODEL_PARAMETER=/gusto/poc/model-key", "DEEPSEEK_PARAMETER=/gusto/poc/deepseek-key", "LEASE_MINUTES=30", "MAX_HOURS=4",
		"CELL_SECRET_PARAMETER=/gusto/poc/cell-secret", "TURNSTILE_PARAMETER=/gusto/poc/turnstile-secret",
		"LEAD_HOOK_URL_PARAMETER=/gusto/poc/lead-hook-url", "LEAD_HOOK_SECRET_PARAMETER=/gusto/poc/lead-hook-secret",
	}, "\n")+"\n"), 0o644)
	h.env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GUSTO_CONF="+h.conf, "GUSTO_RUN="+filepath.Join(dir, "run"), "GUSTO_LE="+filepath.Join(dir, "le"), "GUSTO_LIB="+filepath.Join(dir, "lib"),
		"GUSTO_OWNER=", "GUSTO_LEASE_FILE="+filepath.Join(dir, "lease", "lease.json"), "CREDENTIALS="+filepath.Join(bin, "credentials.sh"), "FAKE_STORE="+h.store, "FAKE_LOG="+h.log)
	return h
}

func (h *host) put(key, text string) {
	h.t.Helper()
	path := filepath.Join(h.store, key)
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// boot runs the script and returns what it did, one line for each command that changed something.
func (h *host) boot(extra ...string) ([]string, error) {
	h.t.Helper()
	os.Remove(h.log)
	command := exec.Command("bash", script)
	command.Env = append(h.env, extra...)
	output, err := command.CombinedOutput()
	if err != nil {
		h.t.Logf("%s", output)
	}
	text, _ := os.ReadFile(h.log)
	var done []string
	for _, line := range strings.Split(strings.TrimSpace(string(text)), "\n") {
		if line != "" && !strings.HasPrefix(line, "aws ssm get-parameter") {
			done = append(done, line)
		}
	}
	return done, err
}

func count(lines []string, prefix string) int {
	n := 0
	for _, line := range lines {
		if strings.HasPrefix(line, prefix) {
			n++
		}
	}
	return n
}

func TestAFirstBootIssuesTheCertificateOnceKeepsItInTheStoreAndPointsTheNameLast(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "the image")
	done, err := h.boot()
	if err != nil {
		t.Fatal(err)
	}
	if count(done, "certbot ") != 1 || !strings.Contains(strings.Join(done, "\n"), "-d gusto.example.com -d *.gusto.example.com") {
		t.Errorf("one certificate for the name and every name under it: %v", done)
	}
	if _, err := os.Stat(filepath.Join(h.store, "tls", "gusto.example.com.tgz")); err != nil {
		t.Error("the certificate was not kept in the store")
	}
	last := done[len(done)-1]
	if last != "gusto-dns -zone example.com -name gusto.example.com -ip 203.0.113.7 -token-file "+filepath.Join(h.dir, "run", "secrets", "cf-dns-token") {
		t.Errorf("the name is pointed at the host last: %v", done)
	}
	if got, _ := os.ReadFile(filepath.Join(h.dir, "lib", "runner-all-rootfs.tar.gz")); string(got) != "the image" {
		t.Errorf("the image: %q", got)
	}
	if !strings.Contains(strings.Join(done, "\n"), "credentials /gusto/poc/model-key /gusto/poc/deepseek-key "+filepath.Join(h.dir, "run", "creds")) {
		t.Errorf("the credentials are built from the two model keys: %v", done)
	}
}

func TestTheTokensAreKeptWhereOnlyTheOwnerCanReadThem(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "x")
	if _, err := h.boot(); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{
		"cf-dns-token":   "value-of-/gusto/poc/cloudflare-dns-token",
		"certbot-cf.ini": "dns_cloudflare_api_token = value-of-/gusto/poc/cloudflare-dns-token\n",
		"mail.env":       "GUSTO_CLOUD_MAIL_TOKEN=value-of-/gusto/poc/cloudflare-mail-token\n",
	} {
		path := filepath.Join(h.dir, "run", "secrets", file)
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v", file, info, err)
		}
		if got, _ := os.ReadFile(path); string(got) != want {
			t.Errorf("%s holds %q", file, got)
		}
	}
	if info, _ := os.Stat(filepath.Join(h.dir, "run", "secrets")); info.Mode().Perm() != 0o700 {
		t.Errorf("the directory: %v", info.Mode())
	}
}

func TestALaterHostReusesTheCertificateAndTheImageInsteadOfAskingAgain(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "the image")
	if _, err := h.boot(); err != nil {
		t.Fatal(err)
	}
	// A new host: nothing on it, and the store as the first one left it.
	fresh := newHost(t)
	fresh.store = h.store
	fresh.env = append(fresh.env, "FAKE_STORE="+h.store)
	done, err := fresh.boot()
	if err != nil {
		t.Fatal(err)
	}
	if count(done, "certbot ") != 0 {
		t.Errorf("a certificate in the store with time left is not asked for again: %v", done)
	}
	if got, _ := os.ReadFile(filepath.Join(fresh.dir, "lib", "runner-all-rootfs.tar.gz")); string(got) != "the image" {
		t.Errorf("the image: %q", got)
	}
	if count(done, "gusto-dns ") != 1 {
		t.Errorf("%v", done)
	}
}

func TestARestartOnTheSameHostFetchesNothingItAlreadyHas(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "the image")
	if _, err := h.boot(); err != nil {
		t.Fatal(err)
	}
	done, err := h.boot()
	if err != nil {
		t.Fatal(err)
	}
	if count(done, "certbot ") != 0 || count(done, "aws s3 cp") != 0 {
		t.Errorf("nothing is fetched or issued again: %v", done)
	}
}

func TestACertificateNearExpiryInTheStoreIsIssuedAgainAndReplaced(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "x")
	if _, err := h.boot(); err != nil {
		t.Fatal(err)
	}
	// The store's copy is one that is nearly out of time.
	le := filepath.Join(h.dir, "le")
	os.WriteFile(filepath.Join(le, "live", "gusto.example.com", "fullchain.pem"), []byte("EXPIRING\n"), 0o644)
	done, err := h.boot()
	if err != nil {
		t.Fatal(err)
	}
	if count(done, "certbot ") != 1 || count(done, "aws s3 cp") != 1 {
		t.Errorf("issued again and kept again: %v", done)
	}
}

func TestAChangedImageIsFetchedAgain(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "old image")
	h.put("image/key2.tar.gz", "new image")
	if _, err := h.boot(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(h.conf, []byte(strings.Replace(mustRead(t, h.conf), "IMAGE_KEY=key1", "IMAGE_KEY=key2", 1)), 0o644)
	if _, err := h.boot(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(h.dir, "lib", "runner-all-rootfs.tar.gz")); string(got) != "new image" {
		t.Errorf("%q", got)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func TestAMissingTokenStopsTheBootBeforeAnythingIsIssuedOrPointed(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "x")
	done, err := h.boot("FAKE_MISSING=/gusto/poc/cloudflare-mail-token")
	if err == nil {
		t.Fatal("a token that is not stored fails the boot")
	}
	if count(done, "certbot ") != 0 || count(done, "gusto-dns ") != 0 {
		t.Errorf("nothing was issued or pointed: %v", done)
	}
}

func TestAMissingImageStopsTheBootBeforeTheNameIsPointed(t *testing.T) {
	h := newHost(t)
	done, err := h.boot()
	if err == nil {
		t.Fatal("an image that is not in the store fails the boot")
	}
	if count(done, "gusto-dns ") != 0 {
		t.Errorf("a host that cannot run computers does not take the name: %v", done)
	}
}

func leaseOf(t *testing.T, h *host) (deadline, hardMax int64) {
	t.Helper()
	text := mustRead(t, filepath.Join(h.dir, "lease", "lease.json"))
	if _, err := fmt.Sscanf(strings.Join(strings.Fields(text), " "), `{"deadline": %d, "hardMax": %d}`, &deadline, &hardMax); err != nil {
		t.Fatalf("%q: %v", text, err)
	}
	return deadline, hardMax
}

func TestTheFirstBootStartsTheLeaseAndALaterOneDoesNotGiveMoreTime(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "x")
	before := time.Now().Unix()
	if _, err := h.boot(); err != nil {
		t.Fatal(err)
	}
	deadline, hardMax := leaseOf(t, h)
	if deadline < before+30*60 || deadline > time.Now().Unix()+30*60 {
		t.Errorf("thirty minutes from the start: %d, started at %d", deadline, before)
	}
	if hardMax < before+4*3600 || hardMax > time.Now().Unix()+4*3600 {
		t.Errorf("four hours at most: %d", hardMax)
	}
	info, _ := os.Stat(filepath.Join(h.dir, "lease", "lease.json"))
	if info.Mode().Perm() != 0o644 {
		t.Errorf("the front door and the reaper both read it: %v", info.Mode())
	}
	// Somebody extended it, and the script ran again.
	extended := fmt.Sprintf(`{"deadline": %d, "hardMax": %d}`, deadline+1800, hardMax)
	os.WriteFile(filepath.Join(h.dir, "lease", "lease.json"), []byte(extended), 0o644)
	time.Sleep(1100 * time.Millisecond)
	if _, err := h.boot(); err != nil {
		t.Fatal(err)
	}
	if again, _ := leaseOf(t, h); again != deadline+1800 {
		t.Errorf("a second boot changed the lease: %d, wanted %d", again, deadline+1800)
	}
}
