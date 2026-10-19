// Command shared runs two different harnesses, one after the other, in one
// OpenShell sandbox on one project, and shows what each turn changed. It is a
// test for the instance in deploy/aws/openshell, not part of gusto: it uses the
// packages a server would, and keeps the sandbox for the next steps of the test.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/getlatentic/gusto/internal/harness"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/proxy"
	"github.com/getlatentic/gusto/internal/spend"
)

const (
	id       = "shared-exp"
	proxyOn  = "127.0.0.1:8041"
	viaProxy = "http://host.openshell.internal:8041/model/v1"
	onHost   = "http://127.0.0.1:8041/model/v1"
)

// turns is what each harness is asked, on the model that serves it.
var turns = []struct{ harness, model, brief string }{
	{"claude", "anthropic/deepseek-flash", "Add server.js to this project: a Node web server that uses only the built-in http module and listens on the port in the PORT environment variable, or 8080, on 0.0.0.0 (this machine has no IPv6 dual-stack, so bind IPv4 explicitly, and do not use the name localhost). GET / answers an HTML page titled Invoice total that shows the total of the lines [{price:10,quantity:2},{price:5,quantity:1}], worked out by total() from invoice.js. Install no packages and change no other file."},
	{"codex", "deepseek/deepseek-flash", "Two changes, and nothing else. The discount in invoice.js is a hundred times too large: fix it so the tests in this project pass. Then add a GET /health route to server.js that answers ok."},
}

func main() {
	credentials := flag.String("credentials", "creds", "the directory of credentials the proxy holds")
	project := flag.String("project", "sample", "the project every turn works on")
	gateway := flag.String("gateway", "spike", "the gateway's name")
	image := flag.String("image", "", "the root file system the sandbox is made from")
	flag.Parse()
	if err := run(*credentials, *project, *gateway, *image); err != nil {
		fmt.Fprintln(os.Stderr, "shared:", err)
		os.Exit(1)
	}
}

func run(credentialsDir, project, gateway, image string) error {
	held, err := proxy.OpenCredentials(credentialsDir)
	if err != nil {
		return err
	}
	tokens := proxy.NewTokens()
	records := os.TempDir()
	where := func(turn string) (string, bool) { return filepath.Join(records, turn+"-proxy.jsonl"), true }
	stop, err := proxy.New(tokens, held, where, spend.NewLedger(spend.Policy{}), proxy.Allowlist{}).ListenTCP(proxyOn)
	if err != nil {
		return err
	}
	defer stop()
	workspace, err := stage(project)
	if err != nil {
		return err
	}
	cfg := openshell.Config{Binary: "openshell", Gateway: gateway, Image: image, Host: "host.openshell.internal", CPU: "2", Memory: "3Gi"}
	for n, turn := range turns {
		if err := oneTurn(cfg, tokens, workspace, n, turn.harness, turn.model, turn.brief); err != nil {
			return fmt.Errorf("turn %d (%s): %w", n+1, turn.harness, err)
		}
	}
	fmt.Printf("\nthe sandbox %s is left running for the next steps\n", openshell.Name(id))
	return nil
}

// stage is a copy of the project as a git repository at a directory named
// workspace, which is what a sandbox takes.
func stage(project string) (string, error) {
	parent, err := os.MkdirTemp("", "shared-")
	if err != nil {
		return "", err
	}
	workspace := filepath.Join(parent, "workspace")
	steps := [][]string{{"cp", "-R", project, workspace}, {"git", "-C", workspace, "init", "-q"}, {"git", "-C", workspace, "add", "-A"},
		{"git", "-C", workspace, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "start"}}
	for _, step := range steps {
		if out, err := exec.Command(step[0], step[1:]...).CombinedOutput(); err != nil {
			return "", fmt.Errorf("%v: %w: %s", step, err, out)
		}
	}
	return workspace, nil
}

// oneTurn runs one harness in the sandbox. The first turn makes the sandbox and
// the later ones find it by its name, each with a token of its own, so what each
// turn spent is its own in the proxy's record.
func oneTurn(cfg openshell.Config, tokens *proxy.Tokens, workspace string, n int, name, model, brief string) error {
	adapter, err := harness.Load("", name)
	if err != nil {
		return err
	}
	credential, _ := proxy.Named(model)
	token, err := tokens.Mint(fmt.Sprintf("turn-%d", n+1), credential)
	if err != nil {
		return err
	}
	box, err := openshell.New(cfg, id, adapter, viaProxy, onHost, token)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if n == 0 {
		if err := box.Start(ctx, workspace); err != nil {
			return err
		}
	}
	if err := box.Equip(ctx, job.Dir(os.TempDir()), job.Spec{}, adapter); err != nil {
		return err
	}
	// An upload leaves .git behind, so the sandbox's copy gets a history of its own to diff turns against.
	inside(ctx, box, "the state before turn", "[ -d .git ] || git init -q; git add -A; git -c user.email=t@t -c user.name=t commit -qm before --allow-empty")
	spec := job.Spec{Brief: brief, Harness: name, Model: model}
	command := box.Wrap(ctx, adapter.CommandIn(ctx, spec, box.Workdir(), box.Env()))
	var said bytes.Buffer
	command.Stdout, command.Stderr = &said, &said
	started := time.Now()
	err = command.Run()
	fmt.Printf("\n== turn %d: %s on %s: %s in %s\n%s\n", n+1, name, model, status(err), time.Since(started).Round(time.Second), tail(said.String(), 900))
	inside(ctx, box, "what this turn changed", "git add -A; git -c user.email=t@t -c user.name=t commit -qm after; git diff --stat HEAD~1 HEAD; git diff HEAD~1 HEAD | head -60")
	return nil
}

func inside(ctx context.Context, box *openshell.Sandbox, label, script string) {
	command := box.Wrap(ctx, exec.Command("sh", "-c", "cd "+box.Workdir()+" && "+script))
	out, err := command.CombinedOutput()
	fmt.Printf("-- %s (%s)\n%s", label, status(err), out)
}

func status(err error) string {
	if err != nil {
		return "failed: " + err.Error()
	}
	return "ok"
}

func tail(text string, n int) string {
	if len(text) > n {
		return "..." + text[len(text)-n:]
	}
	return text
}
