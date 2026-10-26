// Package openshell runs a task's agent in an OpenShell sandbox, driven
// through the openshell command against a gateway. gusto calls it as a component
// below itself: the sandbox enforces what the agent may reach and write, and
// gusto keeps the record, so the record is written outside the sandbox and the
// sandbox's own log of what it allowed and denied goes into it.
package openshell

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// Config says where the gateway is and what the sandboxes are made from.
type Config struct {
	// Binary is the openshell command.
	Binary string
	// Gateway is the registered gateway to use, or empty for the selected one.
	Gateway string
	// Image is what a sandbox is made from: an image reference the gateway can
	// pull, or a root file system archive it can stage. It carries the agent.
	// Empty is the gateway's own default image, which carries none.
	Image string
	// Host is the name a sandbox uses for the machine the gateway runs on.
	Host string
	// Allowed are the hosts a task may reach besides its model, as gusto
	// configures them: a host, or host:port, or *.domain.
	Allowed []string
	// CPU and Memory limit a sandbox, in OpenShell's forms ("2", "4Gi").
	CPU, Memory string
	// Processes is the most processes an agent's user may have running in a sandbox at once. The VM driver
	// enforces no limit of its own, so the command that starts an agent sets it.
	Processes int
}

// defaultProcesses leaves room for an agent, its tools and a test runner's workers, and stops a fork that never ends.
const defaultProcesses = 512

// unsetProcesses is the limit for a sandbox whose configuration names none.
var unsetProcesses = defaultProcesses

// FromEnv reads the settings of a deployment that runs tasks in sandboxes.
func FromEnv() (Config, error) {
	cfg := Config{
		Binary:  or(os.Getenv("GUSTO_OPENSHELL"), "openshell"),
		Gateway: os.Getenv("GUSTO_OPENSHELL_GATEWAY"),
		Image:   os.Getenv("GUSTO_OPENSHELL_IMAGE"),
		Host:    or(os.Getenv("GUSTO_OPENSHELL_HOST"), "host.openshell.internal"),
		CPU:     or(os.Getenv("GUSTO_OPENSHELL_CPU"), "1"),
		Memory:  or(os.Getenv("GUSTO_OPENSHELL_MEMORY"), "3Gi"),
	}
	processes, err := strconv.Atoi(or(os.Getenv("GUSTO_OPENSHELL_PROCESSES"), strconv.Itoa(defaultProcesses)))
	if err != nil || processes < 1 {
		return cfg, fmt.Errorf("GUSTO_OPENSHELL_PROCESSES is %q, and has to be a number of processes above zero", os.Getenv("GUSTO_OPENSHELL_PROCESSES"))
	}
	cfg.Processes = processes
	for _, entry := range strings.Split(os.Getenv("GUSTO_ALLOWED_HOSTS"), ",") {
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

// Sandbox is one task's sandbox. It implements runner.Box.
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
	// A workspace is a sandbox that outlives a task: it is made once, each task works in a
	// directory of its own in it, and the task's part of the guard's log is what came
	// after the lines that were there when it began.
	persistent bool
	// keepDir is a directory that is left when the task ends: a conversation's, which its
	// next turn works in.
	keepDir bool
	workdir string
	job     string
	logBase int
}

// New prepares the sandbox for a task. modelURL is where the sandbox reaches the
// gusto proxy, on the name the sandbox gives the gateway's machine, and token
// stands in for the credential the proxy holds.
func New(cfg Config, id string, adapter harness.Adapter, modelURL, hostModelURL, token string) (*Sandbox, error) {
	s := &Sandbox{cfg: cfg, name: nameOf(id), model: modelURL, hostModel: hostModelURL, token: token}
	if adapter.Keeps != "" {
		s.keeps = append(s.keeps, harness.Rehome(adapter.Keeps, Home))
	}
	policy, err := ComputerPolicy(cfg, modelURL, adapter)
	if err != nil {
		return nil, err
	}
	s.policy = policy
	return s, nil
}

// jobName is what a directory in a workspace may be called.
var jobName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,80}$`)

// dirName is a task's id as a directory's name in a workspace. A name of plain letters,
// digits and hyphens is itself. Any other, with a letter from another alphabet or
// a slash or dots in it, is made plain and given a digest of the whole, so it cannot
// reach outside its directory and two that are alike stay two.
func dirName(id string) string {
	if jobName.MatchString(id) {
		return id
	}
	var plain strings.Builder
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			plain.WriteRune(r)
		case plain.Len() > 0 && !strings.HasSuffix(plain.String(), "-"):
			plain.WriteByte('-')
		}
	}
	sum := sha256.Sum256([]byte(id))
	name := strings.Trim(plain.String(), "-")
	if len(name) > 60 {
		name = strings.Trim(name[:60], "-")
	}
	if name != "" {
		name += "-"
	}
	return name + hex.EncodeToString(sum[:])[:8]
}

// NewComputer prepares a task in a workspace: a sandbox that lasts, called name, that this
// task works in a directory of. The sandbox is made when it is not there, and is
// not removed when the task ends.
//
// A turn in a conversation is given the instance it is a turn on. The agent ties a
// conversation to the directory it began in, so every turn of an instance works in the
// same directory of the workspace, which is kept between turns. Any other task works in one
// of its own, which goes when the task does.
func NewComputer(cfg Config, name, job, instance string, adapter harness.Adapter, modelURL, hostModelURL, token string) (*Sandbox, error) {
	s, err := New(cfg, job, adapter, modelURL, hostModelURL, token)
	if err != nil {
		return nil, err
	}
	s.name, s.persistent, s.job = name, true, dirName(job)
	if instance != "" {
		s.job, s.keepDir = dirName("instance-"+instance), true
	}
	s.workdir = jobsDir + "/" + s.job + "/workspace"
	return s, nil
}

// jobsDir is where the tasks of a workspace are, each in a directory of its own.
const jobsDir = Home + "/jobs"

// Home is the home of the user a sandbox runs the agent as.
const Home = "/sandbox"

// Equip puts the task's skills in the folder the agent finds them in, each in a
// folder of its own name. The skills were written by this server, and the
// sandbox is only handed them.
func (s *Sandbox) Equip(ctx context.Context, dir job.Dir, spec job.Spec, adapter harness.Adapter) error {
	// An agent's own configuration is part of the image, under /opt/gusto/harness
	// where the sandbox user can read it, and belongs in the home the agent will
	// look in. An image without it has nothing to copy, and that is not an error.
	place := s.run(ctx, "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--",
		"sh", "-c", placeConfig("/opt/gusto/harness", Home))
	if _, err := output(place); err != nil {
		return err
	}
	// An agent's state folder has to be there before it starts, as it is in a
	// container: codex refuses a home that does not exist.
	if adapter.Keeps != "" {
		keep := s.run(ctx, "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--",
			"sh", "-c", `mkdir -p "$1"`, "sh", harness.Rehome(adapter.Keeps, Home))
		if _, err := output(keep); err != nil {
			return err
		}
	}
	// What an earlier task in a workspace put where the agent looks for skills is not
	// this task's.
	if s.persistent && adapter.Skills != "" {
		if _, err := output(s.run(ctx, "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--", "rm", "-rf", harness.Rehome(adapter.Skills, Home))); err != nil {
			return err
		}
	}
	if adapter.Skills == "" {
		return nil
	}
	if _, err := output(s.run(ctx, "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--",
		"sh", "-c", equipGuidance, "sh", harness.Rehome(adapter.Skills, Home), networkGuidance)); err != nil {
		return err
	}
	if spec.Setup == nil {
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
		if _, err := output(s.run(ctx, "sandbox", "upload", s.name, from, harness.Rehome(adapter.Skills, Home))); err != nil {
			return err
		}
	}
	return nil
}

// placeConfig is the shell that copies the image's agent configuration from
// one folder into a home, and points what the copies say of the runner's home at
// this one. An agent reads paths from its own configuration files, where the
// rewrite of a command line does not reach.
func placeConfig(from, home string) string {
	return fmt.Sprintf(`[ -d %[1]s ] || exit 0
mkdir -p %[2]s/.config && cp -R %[1]s/. %[2]s/.config/ && cd %[1]s || exit 1
find . -type f | while IFS= read -r file; do
  grep -q /root/ "$file" && sed 's|/root/|%[2]s/|g' "$file" > %[2]s/.config/"$file.copy" && mv %[2]s/.config/"$file.copy" %[2]s/.config/"$file"
done
exit 0`, from, home)
}

// nameOf is the sandbox's name. A gateway takes at most 19 characters and a task's
// name is longer, so the name is a fixed-length digest of it: the same task is
// always the same sandbox, and two tasks cannot be.
func nameOf(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "gusto-" + hex.EncodeToString(sum[:6])
}

// Name is the sandbox a task runs in, for a person looking for it on the gateway.
func Name(id string) string { return nameOf(id) }

// Env is what an agent in the sandbox is started with: where the proxy is and
// the token that stands in for a credential. Nothing of this process's
// environment goes in.
func (s *Sandbox) Env() harness.Env {
	return harness.Env{
		Boxed: true,
		// The two variables a container is started with, so an agent that reads
		// them finds them where it does there.
		Base:     []string{"GUSTO_MODEL_URL=" + s.model, "GUSTO_MODEL_TOKEN=" + s.token},
		ModelURL: s.model, ModelToken: s.token,
	}
}

func (s *Sandbox) Workdir() string {
	if s.workdir != "" {
		return s.workdir
	}
	return Workdir
}

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

// Start makes the sandbox, with its policy, and puts the workspace in it. A workspace
// is made only when it is not there, and is started again when it was stopped.
func (s *Sandbox) Start(ctx context.Context, workspace string) error {
	if s.persistent {
		return s.startInComputer(ctx, workspace)
	}
	return s.makeSandbox(ctx, workspace)
}

func (s *Sandbox) makeSandbox(ctx context.Context, workspace string) error {
	policy, err := os.CreateTemp("", "gusto-sandbox-policy-")
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
	// A new conversation's workspace holds nothing but its git directory, which an upload leaves behind, and an upload that selects
	// nothing is refused: the folder is made in the sandbox instead.
	if empty, err := holdsNothing(absolute); err != nil {
		return err
	} else if empty {
		_, err = output(s.run(ctx, "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--", "mkdir", "-p", Workdir))
		return err
	}
	_, err = output(s.run(ctx, "sandbox", "upload", s.name, absolute, filepath.Dir(Workdir)))
	return err
}

// holdsNothing says a folder has no file or folder in it but .git.
func holdsNothing(folder string) (bool, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Name() != ".git" {
			return false, nil
		}
	}
	return true, nil
}

// slotsBusy is in what a gateway says when it is already staging as many root
// file system archives as it will at once, which is about four. Sandboxes that
// exist are not affected, and the same request a moment later is accepted.
const slotsBusy = "rootfs tar staging slot"

// How long a sandbox is asked for again when the gateway's slots are busy, and
// how long to wait between asks.
var retryFor, retryEvery = 2 * time.Minute, time.Second

// create asks the gateway for the sandbox, again and again while its staging
// slots are busy. A task is not to fail because five others started with it.
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

// launcher starts a command with the environment it reads from the first lines of its standard input, up to an empty line, and
// gives the command the rest. The environment holds the task's token, and an argument of a process on the host is readable by
// every user of the host.
func (s *Sandbox) launcher() string {
	return s.limit() + `while IFS= read -r entry && [ -n "$entry" ]; do export "$entry"; done; exec "$@"`
}

// limit is shell that bounds the processes the agent's user may have, before anything of the agent starts. bash names the
// limit -u and dash, Debian's sh, names it -p. A sandbox where neither works runs nothing.
func (s *Sandbox) limit() string {
	n := s.cfg.Processes
	if n < 1 {
		n = unsetProcesses
	}
	return fmt.Sprintf(`{ ulimit -u %d 2>/dev/null || ulimit -p %d; } || { echo "gusto: the sandbox's process limit could not be set" >&2; exit 126; }; `, n, n)
}

// carried is the command's environment as the lines launcher reads, followed by what the command itself is given.
func carried(env []string, rest io.Reader) (io.Reader, error) {
	var lines strings.Builder
	for _, entry := range env {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if strings.ContainsAny(entry, "\n\r") {
			return nil, fmt.Errorf("the environment variable %s holds a line break, which cannot be carried into the sandbox", name)
		}
		lines.WriteString(harness.Rehome(entry, Home) + "\n")
	}
	lines.WriteString("\n")
	if rest == nil {
		return strings.NewReader(lines.String()), nil
	}
	return io.MultiReader(strings.NewReader(lines.String()), rest), nil
}

// Wrap runs a command in the sandbox. Its environment, which holds the task's token, goes in on standard input ahead of the
// command's own (see launcher), so it is never an argument on the host. --env would put it in the arguments.
func (s *Sandbox) Wrap(ctx context.Context, command *exec.Cmd) *exec.Cmd {
	args := []string{"sandbox", "exec", "--name", s.name, "--workdir", s.Workdir(), "--no-login-shell", "--no-tty", "--", "sh", "-c", s.launcher(), "sh"}
	for _, argument := range command.Args {
		args = append(args, harness.Rehome(argument, Home))
	}
	wrapped := s.run(ctx, args...)
	wrapped.Stdin, wrapped.Err = carried(command.Env, command.Stdin)
	return wrapped
}

// Attach runs a command in the sandbox over ssh through the gateway. exec
// carries standard input as one piece when it ends, so an agent that is spoken
// to as it works would never hear the first message.
func (s *Sandbox) Attach(ctx context.Context, command *exec.Cmd) (*exec.Cmd, error) {
	if err := s.reachable(ctx); err != nil {
		return nil, err
	}
	// The file is read and removed first, so it is not left when anything after it fails.
	var words []string
	if len(command.Env) > 0 {
		file, err := s.leaveEnv(ctx, command.Env)
		if err != nil {
			return nil, err
		}
		words = append(words, ". "+quoted(file)+"; rm -f "+quoted(file)+";")
	}
	words = append(words, "cd "+quoted(s.Workdir())+" || exit 1;", s.limit(), "exec")
	for _, argument := range command.Args {
		words = append(words, quoted(harness.Rehome(argument, Home)))
	}
	attached := exec.CommandContext(ctx, "ssh", "-F", s.sshConfig, "-T", "-o", "BatchMode=yes", s.sshHost, strings.Join(words, " "))
	attached.Env = cleanEnv()
	attached.Stdin = command.Stdin
	return attached, nil
}

// leaveEnv writes the environment into a file in the sandbox that only its user may read, as lines a shell loads, and says where.
// It goes in on standard input, so the token in it is never an argument on the host. The command that loads it removes it.
func (s *Sandbox) leaveEnv(ctx context.Context, env []string) (string, error) {
	name := make([]byte, 8)
	if _, err := rand.Read(name); err != nil {
		return "", err
	}
	file := "/tmp/gusto-env-" + hex.EncodeToString(name)
	var lines strings.Builder
	for _, entry := range env {
		if strings.Contains(entry, "=") {
			lines.WriteString("export " + quoted(harness.Rehome(entry, Home)) + "\n")
		}
	}
	write := s.run(ctx, "sandbox", "exec", "--name", s.name, "--no-login-shell", "--no-tty", "--", "sh", "-c", `umask 077 && cat > "$1"`, "sh", file)
	write.Stdin = strings.NewReader(lines.String())
	if _, err := output(write); err != nil {
		return "", err
	}
	return file, nil
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
	file, err := os.CreateTemp("", "gusto-sandbox-ssh-")
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
	if _, err := output(s.run(context.Background(), "sandbox", "download", s.name, s.Workdir(), staging)); err != nil {
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
	events = s.jobEvents(events)
	policy, err = output(s.run(context.Background(), "policy", "get", s.name, "--full"))
	if err != nil {
		return events, s.policy, nil
	}
	return events, policy, nil
}

// Close removes the sandbox, and a task's directory in a workspace and no more of it.
func (s *Sandbox) Close() error {
	if s.sshConfig != "" {
		os.Remove(s.sshConfig)
	}
	if s.persistent {
		return s.endJob()
	}
	_, err := output(s.run(context.Background(), "sandbox", "delete", s.name))
	return err
}
