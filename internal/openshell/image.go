package openshell

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
)

// Image is the sandbox image as the catalog asks about it: which programs a task
// in it would find. A sandbox is made from it, asked once, and removed. The
// image is a root file system and carries no label, so nothing is declared and
// what is on the path is all that is known.
func Image(cfg Config) catalog.Image { return image{cfg: cfg} }

type image struct{ cfg Config }

func (i image) Carried(programs []string) catalog.Carried {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	probe := &Sandbox{cfg: i.cfg, name: nameOf("survey-" + fmt.Sprint(time.Now().UnixNano()))}
	defer probe.Close()
	// A sandbox made to be asked what it carries reaches nothing.
	denied, err := denying()
	if err != nil {
		return catalog.Carried{Unanswered: err}
	}

	policy, err := os.CreateTemp("", "gusto-survey-policy-")
	if err != nil {
		return catalog.Carried{Unanswered: err}
	}
	defer os.Remove(policy.Name())
	policy.Write(denied)
	policy.Close()

	args := []string{"sandbox", "create", "--name", probe.name, "--policy", policy.Name(), "--no-tty", "--detach", "--no-auto-providers"}
	if i.cfg.Image != "" {
		args = append(args, "--from", i.cfg.Image)
	}
	if _, err := output(probe.run(ctx, append(args, "--", "sleep", "infinity")...)); err != nil {
		return catalog.Carried{Unanswered: fmt.Errorf("could not start a sandbox to ask what the image carries: %w", err)}
	}
	ask := append([]string{"sandbox", "exec", "--name", probe.name, "--no-login-shell", "--no-tty", "--",
		"sh", "-c", `for program; do command -v "$program" >/dev/null && echo "$program"; done; true`, "sh"}, programs...)
	found, err := output(probe.run(ctx, ask...))
	if err != nil {
		return catalog.Carried{Unanswered: fmt.Errorf("could not ask the sandbox what it carries: %w", err)}
	}
	return catalog.Carried{OnPath: names(string(found))}
}

func names(text string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Fields(text) {
		set[line] = true
	}
	return set
}
