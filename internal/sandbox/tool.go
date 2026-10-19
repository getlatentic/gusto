package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Tool is an MCP server that is a program, which a plugin brought and an administrator allowed, to be run in a box of its own. It is not
// run with the task: the task's agent is not given the program, its files or its credentials, and the program is given nothing of the
// task but what it is asked.
type Tool struct {
	// Job and Name make the container's name.
	Job, Name string
	// Plugin is the plugin's folder on this machine, which the program reads and cannot change. Data is a folder it may write.
	Plugin, Data string
	Command      string
	Args         []string
	Env          map[string]string
	Cwd          string
}

// ToolImage is the image programs run in: GUSTO_TOOLS_IMAGE, or the task image, which carries the interpreters a plugin is likely to
// need. A program that needs more than it has is told so by the container.
func ToolImage() string {
	if image := os.Getenv("GUSTO_TOOLS_IMAGE"); image != "" {
		return image
	}
	return Image()
}

// ToolNetwork is what a program reaches: none by default, and with GUSTO_TOOLS_NETWORK=open the runtime's own network, which an
// administrator allows for a program that must call a service. A program that reaches the network can send what it is given to
// anywhere on it.
func ToolNetwork() string {
	if os.Getenv("GUSTO_TOOLS_NETWORK") == "open" {
		return "bridge"
	}
	return "none"
}

var notName = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// ToolName is the name a program's container has, so that it can be found and removed by it.
func ToolName(job, name string) string {
	return "gusto-tool-" + strings.Trim(notName.ReplaceAllString(job+"-"+name, "-"), "-.")
}

// inside is where the plugin and its folder to write are in the box.
const (
	insidePlugin = "/plugin"
	insideData   = "/data"
)

// ToolArgs is the container's command line. The box has no capability, no network unless allowed, a file system it cannot write but
// for its own folder and a scratch space, an unprivileged user, and a bound on what it takes of the machine. The image's own entry is
// replaced, so that the program is the one the plugin names.
func ToolArgs(tool Tool) ([]string, error) {
	plugin, err := filepath.Abs(tool.Plugin)
	if err != nil {
		return nil, err
	}
	data, err := filepath.Abs(tool.Data)
	if err != nil {
		return nil, err
	}
	command, err := inBox(tool.Command, true)
	if err != nil {
		return nil, err
	}
	expand := strings.NewReplacer("${PLUGIN_ROOT}", insidePlugin, "${PLUGIN_DATA}", insideData)
	args := []string{"run", "--rm", "-i", "--init", "--name", ToolName(tool.Job, tool.Name),
		"--network=" + ToolNetwork(),
		"--read-only", "--tmpfs=/tmp:rw,nosuid,size=64m",
		"--cap-drop=ALL", "--security-opt=no-new-privileges", "--user=65534:65534",
		"--cpus=" + share("GUSTO_TOOL_CPUS", "0.5"), "--memory=" + share("GUSTO_TOOL_MEMORY", "512m"), "--pids-limit=128",
		"-v", plugin + ":" + insidePlugin + ":ro",
		"-v", data + ":" + insideData,
		"-e", "PLUGIN_ROOT=" + insidePlugin, "-e", "PLUGIN_DATA=" + insideData, "-e", "HOME=" + insideData,
	}
	names := make([]string, 0, len(tool.Env))
	for name := range tool.Env {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "PLUGIN_ROOT" || name == "PLUGIN_DATA" || strings.ContainsAny(name, "=\x00") {
			continue
		}
		args = append(args, "-e", name+"="+expand.Replace(tool.Env[name]))
	}
	cwd := insidePlugin
	if tool.Cwd != "" {
		if cwd, err = inBox(expand.Replace(tool.Cwd), false); err != nil {
			return nil, err
		}
	}
	args = append(args, "-w", cwd, "--entrypoint", command, ToolImage())
	for _, arg := range tool.Args {
		args = append(args, expand.Replace(arg))
	}
	return args, nil
}

// inBox is a path in the plugin, written as ./path or from the plugin's root, as it is in the box, or a bare program name left for the
// box's own search. A path that leaves the plugin is refused, though the reader of a plugin has already refused one.
func inBox(written string, program bool) (string, error) {
	switch {
	case strings.HasPrefix(written, "./"):
		written = insidePlugin + "/" + strings.TrimPrefix(written, "./")
	case program && !strings.ContainsAny(written, "/\\"):
		return written, nil
	}
	clean := path.Clean(written)
	if !strings.HasPrefix(clean, insidePlugin+"/") && clean != insidePlugin && !strings.HasPrefix(clean, insideData+"/") && clean != insideData {
		return "", fmt.Errorf("%q is not a path in the plugin or its data", written)
	}
	return clean, nil
}

// ToolCommand is the command that runs a program in a box, for the gateway to speak to over its standard input and output. The second
// result removes the container by name once the program has ended, whatever became of the client that started it.
func ToolCommand(_ context.Context, tool Tool) (*exec.Cmd, func(), error) {
	args, err := ToolArgs(tool)
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(tool.Data, 0o777); err != nil {
		return nil, nil, err
	}
	// The box's user is not the server's, so the folder it writes is open to it.
	if err := os.Chmod(tool.Data, 0o777); err != nil {
		return nil, nil, err
	}
	name := ToolName(tool.Job, tool.Name)
	return exec.Command(Runtime(), args...), func() { _ = exec.Command(Runtime(), "rm", "-f", name).Run() }, nil
}
