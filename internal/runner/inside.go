// Package runner does the work: the agent, then the check, then the record.
//
// This runs inside the sandbox. An agent that drives an external CLI could be
// contained by wrapping that CLI, but an agent with its own tool loop runs its
// edits and shell in its own process — so the boundary goes around the runner,
// and then every agent is isolated the same way.
package runner

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/task"
)

// Place is what a runner is told of where it runs, by the command that starts it.
type Place struct {
	// Signing is who stands behind the record.
	Signing signing.Identity
	// ProxySocket is the proxy's socket in a sealed box, which the agent's loopback is relayed to, or nothing.
	ProxySocket string
	// AgentUID is the user the agent and the check run as, when the runner is root, or 0 for the runner's own.
	AgentUID int
	// KeptHome says the agent's home is a person's own, kept between their tasks.
	KeptHome bool
}

// Inside does the whole run in a place that is nothing but this process, unsigned, and returns the exit code the check
// earned, so a caller needs nothing but the process status to know what happened.
func Inside(dir task.Dir) (int, error) { return InsideContext(context.Background(), dir, Place{}) }

// InsideContext is a run in place that stops early when its context ends, as it does when the budget's seconds run out: the
// agent is killed, what it did is kept, and the manifest says why it stopped.
func InsideContext(parent context.Context, dir task.Dir, place Place) (int, error) {
	return InsideWith(parent, dir, harness.Here(), place)
}

// InsideWith is InsideContext for an agent started with the environment given, and not this process's: a task run in the server's
// own process is given the proxy and its token and nothing of the server's secrets. The check runs with the same environment.
func InsideWith(parent context.Context, dir task.Dir, env harness.Env, place Place) (int, error) {
	stopRelay, err := relayOut(place.ProxySocket)
	if err != nil {
		return 0, err
	}
	defer stopRelay()
	spec, err := task.Read(dir)
	if err != nil {
		return 0, err
	}
	agent := agentIn(place)
	defer agent.takeBack(dir)
	run, err := prepareInside(dir, spec, env)
	if err != nil {
		return 0, err
	}
	end := ending{dir: dir, since: evidence.Now(), harness: run.adapter.Name, digest: run.digest, endpoint: run.endpoint, signing: place.Signing}
	record, err := Open(dir, run.adapter)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(spec.Budget.Seconds)*time.Second)
	defer cancel()
	end.exit, err = run.agentTurn(ctx, &agent, record)
	if end.tokens, end.cost, err = finished(record, err); err != nil {
		return 0, err
	}
	if err := Diff(dir, run.before); err != nil {
		return 0, err
	}
	if end.verdict, err = run.check(ctx, agent); err != nil {
		return 0, err
	}
	end.version, end.session = harness.VersionOf(run.adapter.Name), record.Remembered
	return end.close(ctx)
}

// inside is a run of an agent in this process, as prepareInside readied it.
type inside struct {
	dir     task.Dir
	spec    task.Spec
	env     harness.Env
	adapter harness.Adapter
	// digest is taken from what the task carries, before the agent starts: nothing it writes during the run can change what
	// the manifest says it was.
	digest   string
	endpoint string
	// before is the working folder as the agent was given it, which its changes are diffed against.
	before string
}

// prepareInside readies a run: the agent's adapter, in the home it is given, admitted to the model it names, and the working folder
// checked out and snapshotted. A run that may not reach its model is refused in its record.
func prepareInside(dir task.Dir, spec task.Spec, env harness.Env) (*inside, error) {
	configured, err := harness.Open(dir, spec.Harness)
	if err != nil {
		return nil, err
	}
	adapter := configured.Adapter
	if env.Home != "" {
		adapter = adapter.At(env.Home, env.Data)
	}
	if err := admit(spec, adapter, env); err != nil {
		return nil, Refuse(dir, err)
	}
	run := &inside{dir: dir, spec: spec, env: env, adapter: adapter, digest: configured.Digest, endpoint: adapter.EndpointOf()}
	if err := dir.Make(); err != nil {
		return nil, err
	}
	if err := Fill(spec, dir); err != nil {
		return nil, err
	}
	if run.before, err = Snapshot(dir); err != nil {
		return nil, err
	}
	return run, nil
}

// agentTurn gives the agent its tools and its home, runs it until it ends or ctx does, and returns its exit code.
func (r *inside) agentTurn(ctx context.Context, agent *agent, record *Record) (int, error) {
	servers, err := r.equip(agent)
	if err != nil {
		return 0, err
	}
	command := r.adapter.CommandIn(ctx, r.spec, r.dir.WorkingFolder(), r.env)
	if r.env.Wrap != nil {
		r.env.Wrap(command)
	}
	ownGroup(command)
	// The box does not give its root the right to signal another user's processes, and the harness runs as the agent. So the
	// runner cannot kill it: the agent's own processes are ended as the agent, the way they are at the end of a run.
	released := make(chan struct{})
	defer close(released)
	go func() {
		select {
		case <-ctx.Done():
			agent.endAll()
		case <-released:
		}
	}()
	defer agent.endAll()
	if r.adapter.Stream == harness.ACP {
		if err := agent.handOver(r.dir, command); err != nil {
			return 0, err
		}
		return converse(ctx, command, resolved(command.Dir), r.spec, servers, r.adapter.Authenticate, record)
	}
	if err := r.adapter.Give(command, servers); err != nil {
		return 0, err
	}
	if err := agent.handOver(r.dir, command); err != nil {
		return 0, err
	}
	return stream(ctx, command, record)
}

// equip is the tool servers the agent is given, the task's own and the gateway's, and its home made fresh with its skills. A
// tool server that cannot be given is a refusal, in the record.
func (r *inside) equip(agent *agent) ([]task.Server, error) {
	servers, err := Servers(r.dir)
	if err == nil {
		var more []task.Server
		more, err = gatewayServers(r.spec, r.env.ModelURL, r.env.ModelToken)
		servers = append(servers, more...)
	}
	if err != nil {
		return nil, Refuse(r.dir, err)
	}
	agent.skills = r.adapter.Skills
	if err := agent.freshenHome(); err != nil {
		return nil, err
	}
	if r.adapter.Keeps != "" {
		if err := os.MkdirAll(r.adapter.Keeps, 0o700); err != nil {
			return nil, err
		}
	}
	return servers, placeSkills(r.dir, r.spec, r.adapter)
}

// check asks the task's check, with the agent's environment, and ends whatever the check left running. A turn in a conversation
// names no check: it ran, and the evidence says what happened without claiming anything passed.
func (r *inside) check(ctx context.Context, agent agent) (*evidence.Verdict, error) {
	if len(r.spec.Check) == 0 {
		return nil, nil
	}
	given, err := checkWith(ctx, r.spec, r.dir, r.env.Base, r.env.Wrap, agent)
	agent.endAll()
	if err != nil {
		return nil, err
	}
	return &given, nil
}

type said struct {
	text   string
	stderr bool
}

// stream reads both pipes into one order and keeps every line. The scanner is
// given room because an agent's JSON line is routinely longer than the 64 KiB
// a bufio.Scanner takes by default, and a dropped line is lost evidence.
func stream(ctx context.Context, command *exec.Cmd, record *Record) (int, error) {
	out, err := command.StdoutPipe()
	if err != nil {
		return 0, err
	}
	errs, err := command.StderrPipe()
	if err != nil {
		return 0, err
	}
	if err := command.Start(); err != nil {
		return 0, err
	}
	lines := make(chan said, 256)
	var readers sync.WaitGroup
	readers.Add(2)
	go scan(out, false, lines, &readers)
	go scan(errs, true, lines, &readers)
	go func() {
		readers.Wait()
		close(lines)
	}()
	for line := range lines {
		var err error
		if line.stderr {
			err = record.Err(line.text)
		} else {
			err = record.Out(line.text)
		}
		if err != nil {
			return 0, err
		}
	}
	waitErr := command.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		return command.ProcessState.ExitCode(), nil
	}
	var exit *exec.ExitError
	if waitErr != nil && !asExit(waitErr, &exit) {
		return 0, waitErr
	}
	return command.ProcessState.ExitCode(), nil
}

func scan(pipe io.Reader, stderr bool, lines chan<- said, done *sync.WaitGroup) {
	defer done.Done()
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		lines <- said{text: scanner.Text(), stderr: stderr}
	}
}

func asExit(err error, into **exec.ExitError) bool {
	exit, ok := err.(*exec.ExitError)
	if ok {
		*into = exit
	}
	return ok
}
