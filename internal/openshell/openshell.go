// Package openshell runs a job's harness in an OpenShell sandbox, driven
// through the openshell command against a gateway. haas calls it as a component
// below itself: the sandbox enforces what the agent may reach and write, and
// haas keeps the record, so the record is written outside the sandbox and the
// sandbox's own log of what it allowed and denied goes into it.
package openshell

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
)

// Config says where the gateway is and what the sandboxes are made from.
type Config struct {
	// Binary is the openshell command.
	Binary string
	// Gateway is the registered gateway to use, or empty for the selected one.
	Gateway string
	// Image is what a sandbox is made from: an image reference the gateway can
	// pull, or a root file system archive it can stage. It carries the harness.
	// Empty is the gateway's own default image, which carries none.
	Image string
	// Host is the name a sandbox uses for the machine the gateway runs on.
	Host string
	// Allowed are the hosts a job may reach besides its model, as haas
	// configures them: a host, or host:port, or *.domain.
	Allowed []string
	// CPU and Memory limit a sandbox, in OpenShell's forms ("2", "4Gi").
	CPU, Memory string
}

// FromEnv reads the settings of a deployment that runs jobs in sandboxes.
func FromEnv() (Config, error) {
	cfg := Config{
		Binary:  or(os.Getenv("HAAS_OPENSHELL"), "openshell"),
		Gateway: os.Getenv("HAAS_OPENSHELL_GATEWAY"),
		Image:   os.Getenv("HAAS_OPENSHELL_IMAGE"),
		Host:    or(os.Getenv("HAAS_OPENSHELL_HOST"), "host.openshell.internal"),
		CPU:     or(os.Getenv("HAAS_OPENSHELL_CPU"), "1"),
		Memory:  or(os.Getenv("HAAS_OPENSHELL_MEMORY"), "3Gi"),
	}
	for _, entry := range strings.Split(os.Getenv("HAAS_ALLOWED_HOSTS"), ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			cfg.Allowed = append(cfg.Allowed, entry)
		}
	}
	return cfg, nil
}

func or(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// Workdir is where the workspace is inside a sandbox. It is under the home a
// sandbox user has, /sandbox: the file rules can only be laid on a path that
// exists when the sandbox starts, so a directory made at the root is refused.
const Workdir = "/sandbox/workspace"

// Sandbox is one job's sandbox. It implements runner.Box.
type Sandbox struct {
	cfg   Config
	name  string
	model string
	// hostModel is the proxy as this machine reaches it.
	hostModel string
	token     string
	policy    []byte
	keeps     []string
	// sshConfig is the file that says how to reach the sandbox with ssh, and
	// sshHost the name it gives it. Both are made when a conversation needs them.
	sshConfig, sshHost string
}

// New prepares the sandbox for a job. modelURL is where the sandbox reaches the
// haas proxy, on the name the sandbox gives the gateway's machine, and token
// stands in for the credential the proxy holds.
func New(cfg Config, id string, adapter harness.Adapter, modelURL, hostModelURL, token string) (*Sandbox, error) {
	s := &Sandbox{cfg: cfg, name: nameOf(id), model: modelURL, hostModel: hostModelURL, token: token}
	if adapter.Keeps != "" {
		s.keeps = append(s.keeps, inHome(adapter.Keeps))
	}
	policy, err := Policy(cfg, modelURL, s.keeps)
	if err != nil {
		return nil, err
	}
	s.policy = policy
	return s, nil
}

// Home is the home of the user a sandbox runs the harness as.
const Home = "/sandbox"

// inHome is where a path an adapter names for the runner's own home, /root, is
// in a sandbox, where the harness's user has a home of its own.
func inHome(path string) string {
	if rest, found := strings.CutPrefix(path, "/root/"); found {
		return Home + "/" + rest
	}
	return path
}

// inSandbox rewrites every mention of the runner's home in a command line, an
// adapter being written once for a container where the harness's home is /root.
func inSandbox(text string) string {
	return runnersHome.ReplaceAllString(text, "${1}"+Home+"${2}")
}

// runnersHome finds /root as a path of its own: at the start of a word, and not
// inside another path such as /usr/root or /rootless.
var runnersHome = regexp.MustCompile(`(^|[^\w./-])/root($|/)`)

// Equip puts the job's skills in the folder the harness finds them in, each in a
// folder of its own name. The skills were written by this server, and the
// sandbox is only handed them.
func (s *Sandbox) Equip(ctx context.Context, dir job.Dir, spec job.Spec, adapter harness.Adapter) error {
	// A harness's own configuration is part of the image, under /opt/haas/harness
	// where the sandbox user can read it, and belongs in the home the harness will
	// look in. An image without it has nothing to copy, and that is not an error.
	place := s.run(ctx, "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--",
		"sh", "-c", placeConfig("/opt/haas/harness", Home))
	if _, err := output(place); err != nil {
		return err
	}
	// A harness's state folder has to be there before it starts, as it is in a
	// container: codex refuses a home that does not exist.
	if adapter.Keeps != "" {
		keep := s.run(ctx, "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--",
			"sh", "-c", `mkdir -p "$1"`, "sh", inHome(adapter.Keeps))
		if _, err := output(keep); err != nil {
			return err
		}
	}
	if spec.Setup == nil || adapter.Skills == "" {
		return nil
	}
	for _, name := range spec.Setup.Skills {
		if !job.Named(name) {
			return fmt.Errorf("%q is not a skill's name", name)
		}
		from, err := filepath.Abs(filepath.Join(dir.SetupDir(), "skills", name))
		if err != nil {
			return err
		}
		if _, err := output(s.run(ctx, "sandbox", "upload", s.name, from, inHome(adapter.Skills))); err != nil {
			return err
		}
	}
	return nil
}

// placeConfig is the shell that copies the image's harness configuration from
// one folder into a home, and points what the copies say of the runner's home at
// this one. A harness reads paths from its own configuration files, where the
// rewrite of a command line does not reach.
func placeConfig(from, home string) string {
	return fmt.Sprintf(`[ -d %[1]s ] || exit 0
mkdir -p %[2]s/.config && cp -R %[1]s/. %[2]s/.config/ && cd %[1]s || exit 1
find . -type f | while IFS= read -r file; do
  grep -q /root/ "$file" && sed 's|/root/|%[2]s/|g' "$file" > %[2]s/.config/"$file.copy" && mv %[2]s/.config/"$file.copy" %[2]s/.config/"$file"
done
exit 0`, from, home)
}

// nameOf is the sandbox's name. A gateway takes at most 19 characters and a job's
// name is longer, so the name is a fixed-length digest of it: the same job is
// always the same sandbox, and two jobs cannot be.
func nameOf(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "haas-" + hex.EncodeToString(sum[:6])
}

// Name is the sandbox a job runs in, for a person looking for it on the gateway.
func Name(id string) string { return nameOf(id) }

// Env is what a harness in the sandbox is started with: where the proxy is and
// the token that stands in for a credential. Nothing of this process's
// environment goes in.
func (s *Sandbox) Env() harness.Env {
	return harness.Env{
		Boxed: true,
		// The two variables a container is started with, so a harness that reads
		// them finds them where it does there.
		Base:     []string{"HAAS_MODEL_URL=" + s.model, "HAAS_MODEL_TOKEN=" + s.token},
		ModelURL: s.model, ModelToken: s.token,
	}
}

func (s *Sandbox) Workdir() string { return Workdir }

// HostModelURL is where the proxy is from the machine that runs the server.
func (s *Sandbox) HostModelURL() string { return s.hostModel }

// run is the openshell command with the environment it needs and no more: the
// server's own secrets stay out of a process that talks to another server.
func (s *Sandbox) run(ctx context.Context, args ...string) *exec.Cmd {
	full := []string{}
	if s.cfg.Gateway != "" {
		full = append(full, "--gateway", s.cfg.Gateway)
	}
	command := exec.CommandContext(ctx, s.cfg.Binary, append(full, args...)...)
	command.Env = cleanEnv()
	return command
}

// cleanEnv keeps what the openshell command needs to find its gateway and its
// certificates, and nothing else.
func cleanEnv() []string {
	var kept []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name == "PATH" || name == "HOME" || strings.HasPrefix(name, "XDG_") || strings.HasPrefix(name, "OPENSHELL_") {
			kept = append(kept, entry)
		}
	}
	return kept
}

// output runs a command and returns what it printed, and on failure what it
// said, named by the first two words after the program so a log says which step.
func output(command *exec.Cmd) ([]byte, error) {
	var out, complaint bytes.Buffer
	command.Stdout, command.Stderr = &out, &complaint
	if err := command.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("openshell %s: %w: %s", step(command.Args), err, strings.TrimSpace(complaint.String()))
	}
	return out.Bytes(), nil
}

func step(args []string) string {
	var words []string
	for i := 1; i < len(args) && len(words) < 2; i++ {
		if args[i] == "--gateway" {
			i++
			continue
		}
		if !strings.HasPrefix(args[i], "-") {
			words = append(words, args[i])
		}
	}
	return strings.Join(words, " ")
}

// Start makes the sandbox, with its policy, and puts the workspace in it.
func (s *Sandbox) Start(ctx context.Context, workspace string) error {
	policy, err := os.CreateTemp("", "haas-sandbox-policy-")
	if err != nil {
		return err
	}
	defer os.Remove(policy.Name())
	if _, err := policy.Write(s.policy); err != nil {
		policy.Close()
		return err
	}
	policy.Close()
	args := []string{"sandbox", "create", "--name", s.name, "--policy", policy.Name(),
		"--cpu", s.cfg.CPU, "--memory", s.cfg.Memory, "--no-tty", "--detach", "--no-auto-providers"}
	if s.cfg.Image != "" {
		args = append(args, "--from", s.cfg.Image)
	}
	if err := s.create(ctx, append(args, "--", "sleep", "infinity")); err != nil {
		return err
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	// An upload puts a directory under its own name, so the workspace goes to
	// the sandbox's home and lands at Workdir.
	if filepath.Base(absolute) != filepath.Base(Workdir) {
		return fmt.Errorf("the workspace is %s, and a sandbox names it %s", absolute, filepath.Base(Workdir))
	}
	_, err = output(s.run(ctx, "sandbox", "upload", s.name, absolute, filepath.Dir(Workdir)))
	return err
}

// slotsBusy is in what a gateway says when it is already staging as many root
// file system archives as it will at once, which is about four. Sandboxes that
// exist are not affected, and the same request a moment later is accepted.
const slotsBusy = "rootfs tar staging slot"

// How long a sandbox is asked for again when the gateway's slots are busy, and
// how long to wait between asks.
var retryFor, retryEvery = 2 * time.Minute, time.Second

// create asks the gateway for the sandbox, again and again while its staging
// slots are busy. A job is not to fail because five others started with it.
func (s *Sandbox) create(ctx context.Context, args []string) error {
	deadline := time.Now().Add(retryFor)
	for {
		_, err := output(s.run(ctx, args...))
		if err == nil || !strings.Contains(err.Error(), slotsBusy) || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryEvery):
		}
	}
}

// Wrap runs a command in the sandbox. The command's own environment goes in
// front of it as assignments to env, which is why the harness was started with
// an environment of its own. --env would say the same, and warns each time a
// value looks like a credential, and the token here is the one that stands in
// for one.
func (s *Sandbox) Wrap(ctx context.Context, command *exec.Cmd) *exec.Cmd {
	args := []string{"sandbox", "exec", "--name", s.name, "--workdir", Workdir, "--no-login-shell", "--no-tty", "--"}
	if len(command.Env) > 0 {
		args = append(args, "env")
		for _, entry := range command.Env {
			if strings.Contains(entry, "=") {
				args = append(args, inSandbox(entry))
			}
		}
	}
	for _, argument := range command.Args {
		args = append(args, inSandbox(argument))
	}
	wrapped := s.run(ctx, args...)
	wrapped.Stdin = command.Stdin
	return wrapped
}

// Attach runs a command in the sandbox over ssh through the gateway. exec
// carries standard input as one piece when it ends, so an agent that is spoken
// to as it works would never hear the first message.
func (s *Sandbox) Attach(ctx context.Context, command *exec.Cmd) (*exec.Cmd, error) {
	if err := s.reachable(ctx); err != nil {
		return nil, err
	}
	words := []string{"cd " + quoted(Workdir) + " &&", "exec"}
	if len(command.Env) > 0 {
		words = append(words, "env")
		for _, entry := range command.Env {
			if strings.Contains(entry, "=") {
				words = append(words, quoted(inSandbox(entry)))
			}
		}
	}
	for _, argument := range command.Args {
		words = append(words, quoted(inSandbox(argument)))
	}
	attached := exec.CommandContext(ctx, "ssh", "-F", s.sshConfig, "-T", "-o", "BatchMode=yes", s.sshHost, strings.Join(words, " "))
	attached.Env = cleanEnv()
	attached.Stdin = command.Stdin
	return attached, nil
}

// reachable has the gateway say how to ssh to the sandbox, once.
func (s *Sandbox) reachable(ctx context.Context) error {
	if s.sshConfig != "" {
		return nil
	}
	said, err := output(s.run(ctx, "sandbox", "ssh-config", s.name))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(said), "\n") {
		if alias, found := strings.CutPrefix(strings.TrimSpace(line), "Host "); found {
			s.sshHost = alias
			break
		}
	}
	if s.sshHost == "" {
		return fmt.Errorf("openshell sandbox ssh-config named no host for %s", s.name)
	}
	file, err := os.CreateTemp("", "haas-sandbox-ssh-")
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(said); err != nil {
		os.Remove(file.Name())
		return err
	}
	s.sshConfig = file.Name()
	return nil
}

// quoted is text as one word to the shell that runs it in the sandbox.
func quoted(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// Fetch replaces into with the workspace as the sandbox now holds it.
func (s *Sandbox) Fetch(into string) error {
	staging, err := os.MkdirTemp(filepath.Dir(into), ".fetch-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if _, err := output(s.run(context.Background(), "sandbox", "download", s.name, Workdir, staging)); err != nil {
		return err
	}
	// A directory downloads as its contents, links kept as links and not followed,
	// so nothing the agent linked to on this machine is read.
	if err := os.RemoveAll(into); err != nil {
		return err
	}
	return os.Rename(staging, into)
}

// most is how many log lines are asked for. The command's own default is the
// last 200, which would drop the oldest events of a long run without saying so.
const most = "1000000"

// settle is how long the log is given to stop growing. A sandbox ships its log
// to the gateway in batches, so what happened in the last moment of a run is not
// there the moment the run ends.
var (
	settle       = 500 * time.Millisecond
	settleRounds = 20
)

// Guard is what the sandbox's guard recorded, and the policy it enforced. The
// log is read again until two reads agree, so the record holds what the run did
// and not what had reached the gateway when it ended.
func (s *Sandbox) Guard() (events, policy []byte, err error) {
	for round := 0; round < settleRounds; round++ {
		latest, err := output(s.run(context.Background(), "logs", "-n", most, s.name))
		if err != nil {
			return nil, nil, err
		}
		if bytes.Equal(latest, events) && len(latest) > 0 {
			break
		}
		events = latest
		time.Sleep(settle)
	}
	policy, err = output(s.run(context.Background(), "policy", "get", s.name, "--full"))
	if err != nil {
		return events, s.policy, nil
	}
	return events, policy, nil
}

// Close removes the sandbox.
func (s *Sandbox) Close() error {
	if s.sshConfig != "" {
		os.Remove(s.sshConfig)
	}
	_, err := output(s.run(context.Background(), "sandbox", "delete", s.name))
	return err
}
