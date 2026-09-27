// Command haas runs a job.
//
// A job is a directory, running it is this program, and the result is files.
// Every verb takes the directory and nothing else, so the same job runs on a
// laptop, in a managed sandbox, or on a machine the customer owns.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/runner"
	"github.com/getlatentic/haas/internal/sandbox"
	"github.com/getlatentic/haas/internal/signing"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	verb, rest := args[0], args[1:]
	do, found := map[string]func([]string) int{
		"new":       newJob,
		"run":       runJob,
		"inside":    inside,
		"submit":    submit,
		"replay":    replayJobs,
		"compare":   compareJobs,
		"verify":    verify,
		"check":     checkPull,
		"ship":      shipJob,
		"export":    exportJob,
		"show":      show,
		"harnesses": list,
		"serve":     serve,
		"spend":     spendReport,
		"doctor":    checkMachine,
		"keygen":    makeKey,
	}[verb]
	if !found {
		usage()
		return 2
	}
	return do(rest)
}

func usage() {
	fmt.Fprint(os.Stderr, `haas — harness as a service

  haas new <dir> -repo <url> -brief <text> [-harness <name>] -- <gate command>
  haas run <dir>        do the work in a container
  haas inside <dir>     do the work here, without a container
  haas submit <dir>     send the job to HAAS_SERVER and watch it there
  haas verify <dir>     re-hash the evidence, name its signer, re-run the gate [-signed]
  haas ship <dir>       push a passed job's patch and open a pull request [-dry-run]
  haas check <repo> <sha256>  fetch a pull request's evidence and verify it [-into <dir>]
  haas show <dir>       what the job asked for and what happened
  haas replay <dir>...  run finished jobs again at the same commit, compare verdicts [-here]
  haas export <dir>     an audit record anybody can verify [-to, -policy, -requests, -verify]
  haas compare <dir> <harness> <harness>  run one job on each harness, side by side [-here]
  haas serve            the pages: submit a job, watch it, read the record
  haas spend            what the jobs spent this month, by team, project and job [-month 2006-01]
  haas harnesses        the harnesses this binary knows
  haas doctor           check this machine can run a job, before one is sent
  haas keygen <name>    a key pair to sign records with [-dir <dir>]

The exit code of run, inside and verify is the gate's answer.
`)
}

func newJob(args []string) int {
	flags := flag.NewFlagSet("new", flag.ContinueOnError)
	repo := flags.String("repo", "", "the repository to work in")
	brief := flags.String("brief", "", "what to do, in your words")
	which := flags.String("harness", "opencode", "which harness does the work")
	model := flags.String("model", "", "the model to ask for")
	label := flags.String("sensitivity", string(job.Internal), "public, internal or restricted")
	where, rest := split(args)
	if err := flags.Parse(rest); err != nil {
		return 2
	}
	gate := flags.Args()
	if where == "" || *repo == "" || *brief == "" || len(gate) == 0 {
		fmt.Fprintln(os.Stderr, "haas new needs a directory, -repo, -brief, and a gate after --")
		return 2
	}
	dir := job.Dir(where)
	if err := dir.Make(); err != nil {
		return fail(err)
	}
	spec := job.Spec{
		Brief:       *brief,
		Repo:        *repo,
		Gate:        gate,
		Harness:     *which,
		Model:       *model,
		Sensitivity: job.Sensitivity(*label),
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

func runJob(args []string) int {
	dir, code := oneDir(args)
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
	dir, code := oneDir(args)
	if code != 0 {
		return code
	}
	exit, err := runner.Inside(dir)
	if err != nil {
		return fail(err)
	}
	return exit
}

func verify(args []string) int {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	signed := flags.Bool("signed", false, "refuse a record no key signed")
	where, rest := split(args)
	if err := flags.Parse(rest); err != nil || where == "" || flags.NArg() > 0 {
		usage()
		return 2
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
	// The signer is named from the keyring, never from the manifest's signer
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
	if *signed && !attested.Signed {
		return 1
	}
	manifest, err := evidence.Read(dir)
	if err != nil {
		return fail(err)
	}
	spec, err := job.Read(dir)
	if err != nil {
		return fail(err)
	}
	again, _, err := runner.Ask(spec, dir)
	if err != nil {
		return fail(err)
	}
	if manifest.Gate == nil {
		fmt.Println("this job had no gate, so there is no verdict to check")
		return 0
	}
	if again.Passed != manifest.Gate.Passed || again.Code != manifest.Gate.Code {
		fmt.Printf("the gate now exits %d, and the manifest says %d\n", again.Code, manifest.Gate.Code)
		return 1
	}
	fmt.Printf("%d files match, and the gate still exits %d\n", len(manifest.Files), again.Code)
	return 0
}

func show(args []string) int {
	dir, code := oneDir(args)
	if code != 0 {
		return code
	}
	spec, err := job.Read(dir)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("brief    %s\nrepo     %s\nharness  %s\ngate     %s\nlabel    %s\n",
		spec.Brief, spec.Repo, spec.Harness, strings.Join(spec.Gate, " "), spec.Sensitivity)
	manifest, err := evidence.Read(dir)
	if err != nil {
		fmt.Println("\nno manifest yet, so this job has not run here")
		return 0
	}
	// The signer is the keyring's name for the key, as verify gives it, never
	// the manifest's signer field, which is the runner's claim.
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return fail(err)
	}
	fmt.Printf("\ngate     exit %d, passed %t\nharness  exit %d, stopped at the budget %t\nmodel    %s\nsigner   %s\nfiles    %d\n",
		manifest.Gate.Code, manifest.Gate.Passed, manifest.HarnessExit, manifest.StoppedAtBudget,
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

func list(args []string) int {
	for _, name := range harness.Names() {
		adapter, err := harness.Load("", name)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%-10s %-8s needs %s\n", adapter.Name, adapter.Program, adapter.Needs)
	}
	return 0
}

func oneDir(args []string) (job.Dir, int) {
	if len(args) != 1 {
		usage()
		return "", 2
	}
	return job.Dir(args[0]), 0
}

// split takes the directory off the front, so a flag after it is parsed rather
// than silently ignored the way Go's flag package would.
func split(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func fail(err error) int {
	fmt.Fprintf(os.Stderr, "haas: %v\n", err)
	return 2
}
