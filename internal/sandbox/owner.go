package sandbox

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Owner is whose a task's container is: the organization's address, an opaque id of the person (never their email, since anyone
// who may use the runtime reads a container's labels), and the agent's instance. Each is a label on the container, so the
// containers of one organization, person or agent are listed with a filter, and an organization's are removed when its server is.
// An empty field is no label.
type Owner struct {
	Org      string
	Person   string
	Instance string
}

// LabelPrefix begins the name of every label gusto puts on a container.
const LabelPrefix = "com.getlatentic.gusto."

// labels are the flags that label a task's container with its owner and its job.
func (o Owner) labels(job string) []string {
	var args []string
	for _, each := range [][2]string{{"org", o.Org}, {"person", o.Person}, {"instance", o.Instance}, {"job", job}} {
		if each[1] != "" {
			args = append(args, "--label", LabelPrefix+each[0]+"="+each[1])
		}
	}
	return args
}

// removeWithin bounds removing an organization's containers, each of which is given its own stop by the runtime.
const removeWithin = 2 * time.Minute

// RemoveOrg removes every container of the organization's tasks, running or not, with the runtime named. It is for when the
// organization's server has ended: a container whose client was killed with it keeps running, and spending, with nobody to stop it.
func RemoveOrg(runtime, org string) error {
	if org == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), removeWithin)
	defer cancel()
	out, err := exec.CommandContext(ctx, runtime, "ps", "-aq", "--filter", "label="+LabelPrefix+"org="+org).Output()
	if err != nil {
		return fmt.Errorf("listing %s's containers: %w", org, err)
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil
	}
	if out, err := exec.CommandContext(ctx, runtime, append([]string{"rm", "-f"}, ids...)...).CombinedOutput(); err != nil {
		return fmt.Errorf("removing %s's containers: %w: %s", org, err, out)
	}
	return nil
}
