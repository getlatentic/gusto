package openshell

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// A web app an agent built runs in the agent's workspace, in the directory its turns work in. It
// is started as a process that outlives the command that started it, and is given the port
// it must listen on. The gateway then gives it an address of its own, at a host name that
// only the gateway answers to and that has no sign-in, so people do not use that address:
// the organization's server asks it on their behalf, as somebody signed in.

// InstanceDir is where a conversation's agent works in its workspace, and so where an app
// it built is.
func InstanceDir(instance string) string {
	return jobsDir + "/" + dirName("instance-"+instance) + "/workspace"
}

func appFile(id, extension string) string { return Home + "/apps/" + id + "." + extension }

// StartApp starts a command in an agent's directory in a workspace, with PORT and HOST=0.0.0.0
// in its environment, and returns at once. What it writes is kept for AppLog.
func StartApp(ctx context.Context, cfg Config, sandbox, instance, id, command string, port int) error {
	script := `port=$1; log=$2; pidfile=$3; command=$4
mkdir -p "$(dirname "$log")"
own=""; command -v setsid >/dev/null 2>&1 && own=setsid
PORT=$port HOST=0.0.0.0 $own nohup sh -c "$command" > "$log" 2>&1 < /dev/null &
echo $! > "$pidfile"`
	_, err := output((&Sandbox{cfg: cfg}).run(ctx, "sandbox", "exec", "--name", sandbox, "--workdir", InstanceDir(instance), "--no-login-shell", "--no-tty", "--",
		"sh", "-c", script, "sh", fmt.Sprint(port), appFile(id, "log"), appFile(id, "pid"), command))
	return err
}

// StopApp stops what StartApp started, and the processes that started. An app that is not
// running is stopped.
func StopApp(ctx context.Context, cfg Config, sandbox, id string) error {
	script := `pid=$(cat "$1" 2>/dev/null) || exit 0
kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null
rm -f "$1"
exit 0`
	_, err := output((&Sandbox{cfg: cfg}).run(ctx, "sandbox", "exec", "--name", sandbox, "--no-login-shell", "--no-tty", "--",
		"sh", "-c", script, "sh", appFile(id, "pid")))
	return err
}

// AppLog is the end of what an app wrote, for a person who is told it did not answer.
func AppLog(ctx context.Context, cfg Config, sandbox, id string) string {
	text, err := output((&Sandbox{cfg: cfg}).run(ctx, "sandbox", "exec", "--name", sandbox, "--no-login-shell", "--no-tty", "--",
		"sh", "-c", `tail -c 1500 "$1" 2>/dev/null; true`, "sh", appFile(id, "log")))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(text))
}

var exposedAt = regexp.MustCompile(`URL:\s*(\S+)`)

// Address is how the gateway is reached for an exposed service: the host name it answers to, and
// the machine and port to ask.
type Address struct{ Host, Target string }

// ExposeApp gives a port in a workspace an address on the gateway, and says what the address is.
func ExposeApp(ctx context.Context, cfg Config, sandbox string, port int, name string) (Address, error) {
	text, err := output((&Sandbox{cfg: cfg}).run(ctx, "service", "expose", sandbox, fmt.Sprint(port), name))
	if err != nil {
		return Address{}, err
	}
	found := exposedAt.FindSubmatch(text)
	if found == nil {
		return Address{}, fmt.Errorf("the gateway did not say where it exposed the service: %s", strings.TrimSpace(string(text)))
	}
	parsed, err := url.Parse(string(found[1]))
	if err != nil || parsed.Host == "" || parsed.Port() == "" {
		return Address{}, fmt.Errorf("the gateway's address for the service is not one to ask: %s", found[1])
	}
	return Address{Host: parsed.Host, Target: "127.0.0.1:" + parsed.Port()}, nil
}

// UnexposeApp takes an exposed service away. One that is not there is taken away.
func UnexposeApp(ctx context.Context, cfg Config, sandbox, name string) error {
	_, err := output((&Sandbox{cfg: cfg}).run(ctx, "service", "delete", sandbox, name))
	if err != nil && (strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "NotFound")) {
		return nil
	}
	return err
}
