// Command gusto runs a job.
//
// A job is a directory, running it is this program, and the result is files.
// Every verb takes the directory and nothing else, so the same job runs on a
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

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	verb, rest := args[0], args[1:]
	do, found := map[string]func([]string) int{
		"new":         newJob,
		"run":         runJob,
		"inside":      inside,
		"submit":      submit,
		"replay":      replayJobs,
		"compare":     compareJobs,
		"matrix":      matrixJobs,
		"verify":      verify,
		"check":       checkPull,
		"ship":        shipJob,
		"export":      exportJob,
		"show":        show,
		"harnesses":   list,
		"serve":       serve,
		"local":       localCommand,
		"open":        openCommand,
		"cloud":       cloudCommand,
		"cell":        cellCommand,
		"owner":       ownerCommand,
		"survey":      surveyCommand,
		"sandbox-run": sandboxRun,
		"spend":       spendReport,
		"doctor":      checkMachine,
		"keygen":      makeKey,
		"keys":        checkKeys,
		"changes":     changesCommand,
		"retention":   retentionCommand,
		"data":        dataCommand,
		"login":       login,
		"logout":      logout,
		"whoami":      whoami,
		"version":     versionCommand,
	}[verb]
	if !found {
		usage()
		return 2
	}
	return do(rest)
}

func usage() {
	fmt.Fprint(os.Stderr, `gusto — run the coding agents you already use, in a sandbox

  gusto new <dir> -repo <url> -brief <text> [-harness <name>] -- <gate command>
  gusto run <dir>        do the work in a container
  gusto inside <dir>     do the work here, without a container
  gusto submit <dir>     send the job to GUSTO_SERVER and watch it there
  gusto verify <dir>     re-hash the evidence, name its signer, re-run the gate [-signed]
  gusto ship <dir>       push a passed job's patch and open a pull request [-dry-run]
  gusto check <repo> <sha256>  fetch a pull request's evidence and verify it [-into <dir>]
  gusto show <dir>       what the job asked for and what happened
  gusto replay <dir>...  run finished jobs again at the same commit, compare verdicts [-here]
  gusto export <dir>     an audit record anybody can verify [-to, -policy, -requests, -verify]
  gusto compare <dir> <harness> <harness>  run one job on each harness, side by side [-here]
  gusto matrix <plan>    run jobs on harness and model pairs, a table of which finish [-here, -again, -json]
  gusto serve            the pages: submit a job, watch it, read the record
  gusto open             open gusto on this computer in the browser, signed in
  gusto local            gusto for one person on this computer: install, run, stop, uninstall
  gusto cloud            the front door of a hosted gusto: sign-up, sign-in and organisations
  gusto cloud computers  every computer on this host, its organisation and its person
  gusto owner            the owner's page for a hosted gusto, on this machine's loopback only
  gusto cell             the agent on a host that runs organisations' servers for a front door on another
  gusto survey           ask the runner image what it carries and keep the answer beside it, for the servers that start after
  gusto spend            what the jobs spent this month, by team, project and job [-month 2006-01]
  gusto harnesses        the harnesses this binary knows
  gusto doctor           check this machine can run a job, before one is sent
  gusto keygen <name>    a key pair to sign records with [-dir <dir>, -awskms <key>]
  gusto changes verify <file>  check a record of what changed on the platform, and who signed it [-head <hash>]
  gusto retention        what a sweep would remove now, and frees, and removes nothing [-jobs <dir>, -dir <dir>]
  gusto data status|check|rollback -to <release>  the format of each data directory, what starting this release would do to it, and how to go back
  gusto login            sign this computer in as you, by your organisation's server [-server, -label]
  gusto logout           end this computer's sign-in and forget it
  gusto whoami           who this computer is signed in as
  gusto version          which release this is
  gusto keys             which key signed each record, and which no key in the keyring can vouch for [-jobs <dir>]

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
		fmt.Fprintln(os.Stderr, "gusto new needs a directory, -repo, -brief, and a gate after --")
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
	// An interrupt or a termination is somebody stopping the job, whether by the runtime's client or by the server on its way
	// down: the harness is stopped, and the manifest says so.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	exit, err := runner.InsideContext(ctx, dir)
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
	if manifest.Gate == nil {
		fmt.Printf("%d files match, and the job had no gate to run again\n", len(manifest.Files))
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
	gate := "none: a turn in a conversation has no gate"
	if manifest.Gate != nil {
		gate = fmt.Sprintf("exit %d, passed %t", manifest.Gate.Code, manifest.Gate.Passed)
	}
	fmt.Printf("\ngate     %s\nharness  exit %d, stopped at the budget %t\nmodel    %s\nsigner   %s\nfiles    %d\n",
		gate, manifest.HarnessExit, manifest.StoppedAtBudget,
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
	fmt.Fprintf(os.Stderr, "gusto: %v\n", err)
	return 2
}
