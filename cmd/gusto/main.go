// Command gusto runs a task.
//
// A task is a directory, running it is this program, and the result is files.
// Every command takes the directory and nothing else, so the same task runs on a
// laptop, in a managed sandbox, or on a machine the customer owns.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/harness"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/internal/signing"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int { return program().Run(args) }

type newFlags struct{ repo, brief, agent, model, sensitivity string }

func declareNew(set *flag.FlagSet) *newFlags {
	f := &newFlags{}
	set.StringVar(&f.repo, "repo", "", "the `url` of the repository to work in")
	set.StringVar(&f.brief, "brief", "", "what to do, in your `words`")
	set.StringVar(&f.agent, "agent", "opencode", "which agent does the work, from gusto agents")
	set.StringVar(&f.model, "model", "", "the `model` to ask for")
	set.StringVar(&f.sensitivity, "sensitivity", string(job.Internal), "`label` the task: public, internal or restricted")
	return f
}

func newTask(args []string) int {
	set := newSet("new")
	f := declareNew(set)
	where, rest := split(args)
	if code, ok := parseFlags("new", set, rest); !ok {
		return code
	}
	check := set.Args()
	if where == "" || f.repo == "" || f.brief == "" || len(check) == 0 {
		return usageError("new", "needs a directory, -repo, -brief, and a check after --")
	}
	dir := job.Dir(where)
	if err := dir.Make(); err != nil {
		return fail(err)
	}
	spec := job.Spec{
		Brief:       f.brief,
		Repo:        f.repo,
		Gate:        check,
		Harness:     f.agent,
		Model:       f.model,
		Sensitivity: job.Sensitivity(f.sensitivity),
	}
	if err := spec.Write(dir); err != nil {
		return fail(err)
	}
	if err := harness.Keep(dir, spec.Harness); err != nil {
		return fail(err)
	}
	fmt.Println(dir.Spec())
	return 0
}

func runTask(args []string) int {
	dir, code := oneDir("run", args)
	if code != 0 {
		return code
	}
	// The checkout happens here rather than in the container, so a repository
	// credential stays on the runner and never enters the box with the agent.
	spec, err := job.Read(dir)
	if err != nil {
		return fail(err)
	}
	if err := dir.Make(); err != nil {
		return fail(err)
	}
	if err := runner.Fill(spec, dir); err != nil {
		return fail(err)
	}
	exit, err := sandbox.Run(dir, harness.ModelURL(), "")
	if err != nil {
		return fail(err)
	}
	return exit
}

func inside(args []string) int {
	dir, code := oneDir("inside", args)
	if code != 0 {
		return code
	}
	// An interrupt or a termination is somebody stopping the task, whether by the runtime's client or by the server on its way
	// down: the agent is stopped, and the record says so.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	exit, err := runner.InsideContext(ctx, dir)
	if err != nil {
		return fail(err)
	}
	return exit
}

type verifyFlags struct{ signed bool }

func declareVerify(set *flag.FlagSet) *verifyFlags {
	f := &verifyFlags{}
	set.BoolVar(&f.signed, "signed", false, "refuse a record that no key signed")
	return f
}

func verify(args []string) int {
	set := newSet("verify")
	f := declareVerify(set)
	where, rest := split(args)
	if code, ok := parseFlags("verify", set, rest); !ok {
		return code
	}
	if where == "" || set.NArg() > 0 {
		return usageError("verify", "needs one task directory")
	}
	dir := job.Dir(where)
	wrong, err := evidence.Check(dir)
	if err != nil {
		return fail(err)
	}
	for _, complaint := range wrong {
		fmt.Printf("%s\n", complaint)
	}
	if len(wrong) > 0 {
		return 1
	}
	// The signer is named from the keyring, never from the record's signer
	// field. Unsigned passes unless -signed asks otherwise, because every record
	// made before signing, and every run with no key, is unsigned.
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return fail(err)
	}
	attested, err := evidence.Attest(dir, keyring)
	if err != nil {
		fmt.Println(err)
		return 1
	}
	fmt.Println(attested)
	if f.signed && !attested.Signed {
		return 1
	}
	manifest, err := evidence.Read(dir)
	if err != nil {
		return fail(err)
	}
	if manifest.Gate == nil {
		fmt.Printf("%d files match, and the task had no check to run again\n", len(manifest.Files))
		return 0
	}
	spec, err := job.Read(dir)
	if err != nil {
		return fail(err)
	}
	again, _, err := runner.Ask(spec, dir)
	if err != nil {
		return fail(err)
	}
	if again.Passed != manifest.Gate.Passed || again.Code != manifest.Gate.Code {
		fmt.Printf("the check now exits %d, and the record says %d\n", again.Code, manifest.Gate.Code)
		return 1
	}
	fmt.Printf("%d files match, and the check still exits %d\n", len(manifest.Files), again.Code)
	return 0
}

func show(args []string) int {
	dir, code := oneDir("show", args)
	if code != 0 {
		return code
	}
	spec, err := job.Read(dir)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("brief    %s\nrepo     %s\nagent    %s\ncheck    %s\nlabel    %s\n",
		spec.Brief, spec.Repo, spec.Harness, strings.Join(spec.Gate, " "), spec.Sensitivity)
	manifest, err := evidence.Read(dir)
	if err != nil {
		fmt.Println("\nno record yet, so this task has not run here")
		return 0
	}
	// The signer is the keyring's name for the key, as verify gives it, never
	// the record's signer field, which is the runner's claim.
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return fail(err)
	}
	check := "none: a turn in a conversation has no check"
	if manifest.Gate != nil {
		check = fmt.Sprintf("exit %d, passed %t", manifest.Gate.Code, manifest.Gate.Passed)
	}
	fmt.Printf("\ncheck    %s\nagent    exit %d, stopped at the budget %t\nmodel    %s\nsigner   %s\nfiles    %d\n",
		check, manifest.HarnessExit, manifest.StoppedAtBudget,
		manifest.ModelEndpoint, evidence.SignedBy(dir, keyring), len(manifest.Files))
	if manifest.Tokens != nil {
		fmt.Printf("tokens   %d\n", *manifest.Tokens)
	}
	if manifest.CostUSD != nil {
		fmt.Printf("cost     $%.4f\n", *manifest.CostUSD)
	}
	if stop := manifest.StoppedByCap; stop != nil {
		fmt.Printf("stopped  by the %s cap: %s\n", stop.Cap, stop.Reason)
	}
	if tail := runner.Tail(dir.OutFile("gate.log"), 10); tail != "" {
		fmt.Printf("\n%s\n", tail)
	}
	return 0
}

func agentsCommand() int {
	for _, name := range harness.Names() {
		adapter, err := harness.Load("", name)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%-10s %-8s needs %s\n", adapter.Name, adapter.Program, adapter.Needs)
	}
	return 0
}

// split takes the directory off the front, so a flag after it is parsed rather
// than silently ignored the way Go's flag package would.
func split(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}
