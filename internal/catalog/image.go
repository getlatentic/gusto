package catalog

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Label is where the runner image lists the harnesses it carries, as the
// Dockerfile's HARNESSES wrote them: name=package, space-separated.
const Label = "com.getlatentic.haas.harnesses"

// Image is the runner image, as the catalog asks it.
type Image interface {
	// Carried is what the image says it carries and which of these programs
	// it actually has.
	Carried(programs []string) Carried
}

// Carried is one image's answer.
type Carried struct {
	// Declared is the harness names the image's label lists.
	Declared []string
	// OnPath is the programs a job in the image would find.
	OnPath map[string]bool
	// Unasked is an image nobody asked, because there is no container to ask:
	// a job then runs in this process, and the runner itself refuses a
	// program it cannot find.
	Unasked bool
	// Unanswered is why the image could not be asked at all.
	Unanswered error
	// Machine says the programs were looked for on this computer's PATH, where a person installs them themselves, and not in an
	// image.
	Machine bool
}

// Declares reports whether the image names the harness.
func (c Carried) Declares(name string) bool {
	for _, declared := range c.Declared {
		if declared == name {
			return true
		}
	}
	return false
}

// Runs says whether a job on this harness would find its program, and what to
// do when it would not. The label decides only the advice: a program that is
// there runs whatever the label says, and one that is missing does not start
// because the label names it.
func (c Carried) Runs(name, program string) (bool, string) {
	switch {
	case c.Unasked || c.OnPath[program]:
		return true, ""
	case c.Unanswered != nil:
		return false, "the runner image could not be asked what it carries: " + c.Unanswered.Error()
	case c.Declares(name):
		return false, "the runner image says it carries " + name + ", but " + program + " is not on its PATH: rebuild the image"
	}
	return false, "the runner image does not carry " + program + ": add " + name + " to its HARNESSES and rebuild it"
}

// machine is this computer, as a person's own: a job runs here and finds its program on the PATH.
type machine struct{}

// Machine is this computer's PATH, asked in place of an image.
func Machine() Image { return machine{} }

func (machine) Carried(programs []string) Carried {
	found := map[string]bool{}
	for _, program := range programs {
		if _, err := exec.LookPath(program); err == nil {
			found[program] = true
		}
	}
	return Carried{OnPath: found, Machine: true}
}

// Ask runs the container command and hands back what it printed.
type Ask func(args ...string) (string, error)

type container struct {
	ask   Ask
	image string
}

// Container is the image a container runtime would start. With no runtime or
// no image there is nothing to ask, and every program counts as present.
func Container(runtime, image string) Image {
	if runtime == "" || image == "" {
		return unasked{}
	}
	return container{ask: asking(runtime), image: image}
}

// asking gives each question a deadline, so a runtime that hangs holds up a
// server's start by that long and no longer.
func asking(runtime string) Ask {
	return func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, runtime, args...).Output()
		return string(out), err
	}
}

// Carried asks twice: once for the label, which costs no container, and once
// for every program together, in one container rather than one each.
func (c container) Carried(programs []string) Carried {
	label, err := c.ask("image", "inspect", "--format", `{{ index .Config.Labels "`+Label+`" }}`, c.image)
	if err != nil {
		return Carried{Unanswered: fmt.Errorf("%s is not an image here: %w", c.image, err)}
	}
	found, err := c.ask(append([]string{"run", "--rm", "--entrypoint", "sh", c.image,
		"-c", `for program; do command -v "$program" >/dev/null && echo "$program"; done; true`, "sh"},
		programs...)...)
	if err != nil {
		return Carried{Unanswered: fmt.Errorf("could not start %s: %w", c.image, err)}
	}
	return Carried{Declared: Declared(label), OnPath: lines(found)}
}

// Declared reads the names out of a label: the part of each member before its
// equals sign. An image built before the label, which inspect answers with
// nothing or with "<no value>", declares nothing.
func Declared(label string) []string {
	if strings.TrimSpace(label) == "<no value>" {
		return nil
	}
	var names []string
	for _, member := range strings.Fields(label) {
		if name, _, _ := strings.Cut(member, "="); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func lines(text string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Fields(text) {
		set[line] = true
	}
	return set
}

type unasked struct{}

func (unasked) Carried([]string) Carried { return Carried{Unasked: true} }
