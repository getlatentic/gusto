package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/rules"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/box"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/task"
)

// Box is a place an agent runs that this process only drives, such as a
// sandbox on a gateway. It is defined in pkg/box so that code outside this module can
// implement it; the runner stays outside the box and writes the record itself.
type Box = box.Box

// Files the box's guard leaves in the record. They are written before the
// manifest hashes out/, so they are signed with the rest.
const (
	GuardEvents = box.GuardEvents
	GuardPolicy = box.GuardPolicy
)

// Outside does the whole run for an agent that runs in a box, and returns the
// exit code the check earned, as Inside does for one in this process. The
// working folder must already hold the repository.
//
// The record is signed by who signs, and the run stops early when ctx ends, as Inside does.
func Outside(parent context.Context, dir task.Dir, box Box, signs signing.Identity) (int, error) {
	spec, err := task.Read(dir)
	if err != nil {
		return 0, err
	}
	run, err := prepareOutside(dir, spec, box)
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(objectsOf(dir))
	end := ending{dir: dir, since: record.Now(), harness: run.adapter.Name, digest: run.digest, endpoint: run.endpoint, signing: signs}
	recorder, err := Open(dir, run.adapter)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(spec.Budget.Seconds)*time.Second)
	defer cancel()
	// Closed however Start ends: a box that was half made is still there.
	defer box.Close()
	end.exit, err = run.agentTurn(ctx, recorder)
	if end.tokens, end.cost, err = finished(recorder, err); err != nil {
		return 0, err
	}
	if end.verdict, err = run.check(ctx); err != nil {
		return 0, err
	}
	end.version, end.session = versionIn(ctx, box, run.adapter.Name), recorder.Remembered
	return end.close(ctx)
}

// outside is a run of an agent in a box this process drives, as prepareOutside readied it.
type outside struct {
	dir      task.Dir
	spec     task.Spec
	box      Box
	adapter  harness.Adapter
	digest   string
	endpoint string
	before   string
}

// prepareOutside readies a run in a box: what the box cannot carry and a model the run may not reach are refused by name before
// anything starts, and the working folder, which already holds the repository, is snapshotted.
func prepareOutside(dir task.Dir, spec task.Spec, box Box) (*outside, error) {
	if spec.Setup != nil && len(spec.Setup.Servers) > 0 {
		return nil, Refuse(dir, errors.New("a sandbox on a gateway does not yet carry tool servers"))
	}
	configured, err := harness.Open(dir, spec.Harness)
	if err != nil {
		return nil, err
	}
	adapter := configured.Adapter
	endpoint := adapter.EndpointOf()
	if adapter.Needs == harness.NeedsLocal {
		endpoint = box.HostModelURL()
	}
	if err := admitOutside(spec, adapter, endpoint); err != nil {
		return nil, Refuse(dir, err)
	}
	if err := dir.Make(); err != nil {
		return nil, err
	}
	before, err := Snapshot(dir)
	if err != nil {
		return nil, err
	}
	return &outside{dir: dir, spec: spec, box: box, adapter: adapter, digest: configured.Digest, endpoint: endpoint, before: before}, nil
}

// agentTurn starts the box with the working folder, checks it carries the agent, equips it, and talks to the agent until it ends or
// ctx does. A box that does not carry the agent is a refusal, in the record.
func (r *outside) agentTurn(ctx context.Context, record *Recorder) (int, error) {
	if err := r.box.Start(ctx, r.dir.WorkingFolder()); err != nil {
		return 0, err
	}
	if err := carriedIn(ctx, r.box, r.spec.Harness, r.adapter); err != nil {
		return 0, Refuse(r.dir, err)
	}
	if err := r.box.Equip(ctx, r.dir, r.spec, r.adapter); err != nil {
		return 0, err
	}
	return talk(ctx, r.box, r.adapter, r.spec, record)
}

// check brings the working folder out of the box, diffs it, asks the check in the box, and keeps what the box's guard saw.
func (r *outside) check(ctx context.Context) (*record.Verdict, error) {
	if err := r.box.Fetch(r.dir.WorkingFolder()); err != nil {
		return nil, err
	}
	if err := Diff(r.dir, r.before); err != nil {
		return nil, err
	}
	var verdict *record.Verdict
	if len(r.spec.Check) > 0 {
		given, err := checkIn(ctx, r.spec, r.dir, r.box)
		if err != nil {
			return nil, err
		}
		verdict = &given
	}
	return verdict, keepGuard(r.dir, r.box)
}

// checkIn asks the check in the box, where the repository's own code runs, and
// keeps the answer and the whole output.
func checkIn(ctx context.Context, spec task.Spec, dir task.Dir, box Box) (record.Verdict, error) {
	command := box.Wrap(ctx, exec.CommandContext(ctx, spec.Check[0], spec.Check[1:]...))
	command.Dir = ""
	verdict, output, err := askWith(spec, command)
	if err != nil {
		return record.Verdict{}, err
	}
	return verdict, keepCheck(dir, verdict, output)
}

// keepGuard writes what the box's guard recorded into the record.
func keepGuard(dir task.Dir, box Box) error {
	events, policy, err := box.Guard()
	if err != nil {
		return err
	}
	if err := os.WriteFile(dir.OutFile(GuardEvents), events, 0o644); err != nil {
		return err
	}
	return os.WriteFile(dir.OutFile(GuardPolicy), policy, 0o644)
}

// admitOutside is admit for a harness that runs in a box: the program is the
// box's to have, so it is asked of the box once it is up, and the model is
// reached from this process by the address it has for the proxy.
func admitOutside(spec task.Spec, adapter harness.Adapter, endpoint string) error {
	if err := fits(spec, adapter); err != nil {
		return err
	}
	if err := rules.Allows(spec, endpoint); err != nil {
		return err
	}
	if adapter.Needs == harness.NeedsLocal {
		return Reachable(endpoint)
	}
	return nil
}

// carriedIn checks the program a harness runs is on the box's PATH, which is the
// image answering for itself at the moment it matters.
func carriedIn(ctx context.Context, box Box, name string, adapter harness.Adapter) error {
	check := box.Wrap(ctx, exec.CommandContext(ctx, "sh", "-c", `command -v "$1" >/dev/null`, "sh", adapter.Program))
	if err := check.Run(); err != nil {
		return fmt.Errorf("this sandbox's image does not carry %s, the program the %s agent runs: "+
			"build the image with %s in its HARNESSES, or choose an agent the catalog offers",
			adapter.Program, name, name)
	}
	return nil
}

// versionIn is the agent's build as the box's image recorded it when it was installed, or nothing from an image that
// recorded none.
func versionIn(ctx context.Context, box Box, name string) string {
	out, err := box.Wrap(ctx, exec.CommandContext(ctx, "cat", harness.BuildsFile)).Output()
	if err != nil {
		return ""
	}
	return harness.ReadVersions(bytes.NewReader(out))[name]
}

// talk runs the harness in the box and records what it does: an ACP agent is
// held a conversation with, and any other harness's output is read as it comes.
func talk(ctx context.Context, box Box, adapter harness.Adapter, spec task.Spec, record *Recorder) (int, error) {
	command, servers, err := boxCommand(ctx, adapter, spec, box.Workdir(), box.Env())
	if err != nil {
		return 0, err
	}
	if adapter.Stream != harness.ACP {
		return stream(ctx, box.Wrap(ctx, command), record)
	}
	live, err := box.Attach(ctx, command)
	if err != nil {
		return 0, err
	}
	return converse(ctx, live, box.Workdir(), spec, servers, adapter.Authenticate, record)
}

// boxCommand is the command that starts the harness in a box, with the servers it is to be given. A task's servers behind the gateway
// are reached at the proxy by the address the box has for it, the same way the harness reaches its model. A harness that reads its
// servers from a file in its own folder is not given them: the box does not put a file there.
func boxCommand(ctx context.Context, adapter harness.Adapter, spec task.Spec, workingFolder string, env harness.Env) (*exec.Cmd, []task.Server, error) {
	servers, err := gatewayServers(spec, env.ModelURL, env.ModelToken)
	if err != nil {
		return nil, nil, err
	}
	command := adapter.CommandIn(ctx, spec, workingFolder, env)
	if len(servers) == 0 || adapter.Stream == harness.ACP {
		return command, servers, nil
	}
	if adapter.MCP != nil && adapter.MCP.File != "" {
		return nil, nil, fmt.Errorf("%s reads its servers from a file in its own folder, which a sandbox on a gateway does not yet put there", adapter.Name)
	}
	if err := adapter.Give(command, servers); err != nil {
		return nil, nil, err
	}
	return command, servers, nil
}
