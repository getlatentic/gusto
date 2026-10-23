package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/getlatentic/gusto/internal/rules"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// admit decides whether this job can start here at all. Each check is one a
// harness would otherwise fail minutes later, in its own words, and the job
// would read as the harness failing rather than as never having started.
func admit(spec job.Spec, adapter harness.Adapter, env harness.Env) error {
	if err := Carries(spec.Harness, adapter); err != nil {
		return err
	}
	if err := fits(spec, adapter); err != nil {
		return err
	}
	endpoint := adapter.EndpointOf()
	if err := rules.Allows(spec, endpoint); err != nil {
		return err
	}
	// The rules judge where the model is; what is asked is whether the address the harness is given answers, which is the
	// proxy when the job runs in this process.
	if adapter.Needs == harness.NeedsLocal && env.ModelURL != "" {
		return Reachable(env.ModelURL)
	}
	if adapter.Needs == harness.NeedsLocal {
		return Reachable(endpoint)
	}
	return nil
}

// Carries checks the program an agent runs is on this runner's PATH, which is
// the PATH the agent would be started with. Inside a container that is the
// image's, so this is the image answering for itself at the moment it
// matters, whatever the catalog was told when the server started.
func Carries(name string, adapter harness.Adapter) error {
	if _, err := exec.LookPath(adapter.Installed()); err != nil {
		return fmt.Errorf("this runner does not carry %s, the program the %s agent runs: "+
			"build the runner image with %s in its HARNESSES, or choose an agent the catalog offers",
			adapter.Installed(), name, name)
	}
	return nil
}

// Refuse keeps the reason a task never started where a person reading the task
// looks: the transcript, which the task page, the live stream and a
// conversation all show. A run that does start truncates it, so a refusal
// never outlives the next attempt. The reason is still returned, so the
// process exits the way any runner error does.
func Refuse(dir job.Dir, reason error) error {
	said := "gusto refused this task before the agent started: " + reason.Error() + "\n"
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		return errors.Join(reason, err)
	}
	if err := os.WriteFile(dir.OutFile("transcript.log"), []byte(said), 0o644); err != nil {
		return errors.Join(reason, err)
	}
	return reason
}
