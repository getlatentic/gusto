package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/rules"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/box"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
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
// workspace must already hold the repository.
func Outside(dir job.Dir, box Box) (int, error) {
	return OutsideContext(context.Background(), dir, box)
}

// OutsideContext is Outside that stops early when its context ends, as Inside does.
func OutsideContext(parent context.Context, dir job.Dir, box Box) (int, error) {
	key, keyed, err := signing.Configured()
	if err != nil {
		return 0, err
	}
	spec, err := job.Read(dir)
	if err != nil {
		return 0, err
	}
	// What a box cannot yet carry is refused by name before anything starts.
	if spec.Setup != nil && len(spec.Setup.Servers) > 0 {
		return 0, Refuse(dir, errors.New("a sandbox on a gateway does not yet carry tool servers"))
	}
	configured, err := harness.Open(dir, spec.Harness)
	if err != nil {
		return 0, err
	}
	adapter := configured.Adapter
	endpoint := adapter.EndpointOf()
	if adapter.Needs == harness.NeedsLocal {
		endpoint = box.HostModelURL()
	}
	if err := admitOutside(spec, adapter, endpoint); err != nil {
		return 0, Refuse(dir, err)
	}
	if err := dir.Make(); err != nil {
		return 0, err
	}
	before, err := Snapshot(dir)
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(objectsOf(dir))

	started := evidence.Now()
	record, err := Open(dir, adapter)
	if err != nil {
		return 0, err
	}
	budget := time.Duration(spec.Budget.Seconds) * time.Second
	ctx, cancel := context.WithTimeout(parent, budget)
	defer cancel()

	// Closed however Start ends: a box that was half made is still there.
	defer box.Close()
	if err := box.Start(ctx, dir.Workspace()); err != nil {
		record.Close()
		return 0, err
	}
	if err := carriedIn(ctx, box, spec.Harness, adapter); err != nil {
		record.Close()
		return 0, Refuse(dir, err)
	}
	if err := box.Equip(ctx, dir, spec, adapter); err != nil {
		record.Close()
		return 0, err
	}
	exit, err := talk(ctx, box, adapter, spec, record)
	tokens, cost := record.Spent()
	if closeErr := record.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, err
	}

	if err := box.Fetch(dir.Workspace()); err != nil {
		return 0, err
	}
	if err := Diff(dir, before); err != nil {
		return 0, err
	}
	var verdict *evidence.Verdict
	if len(spec.Gate) > 0 {
		given, err := gateIn(ctx, spec, dir, box)
		if err != nil {
			return 0, err
		}
		verdict = &given
	}
	if err := keepGuard(dir, box); err != nil {
		return 0, err
	}
	capped, err := stoppedByCap(dir, started)
	if err != nil {
		return 0, err
	}
	spent, files, err := evidence.Of(dir)
	if err != nil {
		return 0, err
	}
	if err := evidence.Write(dir, evidence.Manifest{
		Job:             spent,
		Files:           files,
		Gate:            verdict,
		Harness:         adapter.Name,
		HarnessDigest:   configured.Digest,
		HarnessVersion:  versionIn(ctx, box, adapter.Name),
		Session:         record.Remembered,
		HarnessExit:     exit,
		StoppedAtBudget: ctx.Err() == context.DeadlineExceeded,
		StoppedBy:       stoppedBy(ctx, dir),
		StoppedByCap:    capped,
		ModelEndpoint:   endpoint,
		Tokens:          tokens,
		CostUSD:         cost,
		Signer:          signer(key, keyed),
		AtUnix:          evidence.Now(),
	}); err != nil {
		return 0, err
	}
	if keyed {
		if err := evidence.Seal(dir, key); err != nil {
			return 0, err
		}
	}
	return earned(verdict, exit), nil
}

// gateIn asks the gate in the box, where the repository's own code runs, and
// keeps the answer and the whole output.
func gateIn(ctx context.Context, spec job.Spec, dir job.Dir, box Box) (evidence.Verdict, error) {
	command := box.Wrap(ctx, exec.CommandContext(ctx, spec.Gate[0], spec.Gate[1:]...))
	command.Dir = ""
	verdict, output, err := askWith(spec, command)
	if err != nil {
		return evidence.Verdict{}, err
	}
	return verdict, keepGate(dir, verdict, output)
}

// keepGuard writes what the box's guard recorded into the record.
func keepGuard(dir job.Dir, box Box) error {
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
func admitOutside(spec job.Spec, adapter harness.Adapter, endpoint string) error {
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
func talk(ctx context.Context, box Box, adapter harness.Adapter, spec job.Spec, record *Record) (int, error) {
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

// boxCommand is the command that starts the harness in a box, with the servers it is to be given. A job's servers behind the gateway
// are reached at the proxy by the address the box has for it, the same way the harness reaches its model. A harness that reads its
// servers from a file in its own folder is not given them: the box does not put a file there.
func boxCommand(ctx context.Context, adapter harness.Adapter, spec job.Spec, workspace string, env harness.Env) (*exec.Cmd, []job.Server, error) {
	servers, err := gatewayServers(spec, env.ModelURL, env.ModelToken)
	if err != nil {
		return nil, nil, err
	}
	command := adapter.CommandIn(ctx, spec, workspace, env)
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
