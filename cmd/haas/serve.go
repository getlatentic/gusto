package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/getlatentic/haas/internal/accesslog"
	"github.com/getlatentic/haas/internal/apitoken"
	"github.com/getlatentic/haas/internal/apps"
	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/compare"
	"github.com/getlatentic/haas/internal/computer"
	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/openshell"
	"github.com/getlatentic/haas/internal/proxy"
	"github.com/getlatentic/haas/internal/quota"
	"github.com/getlatentic/haas/internal/retention"
	"github.com/getlatentic/haas/internal/runner"
	"github.com/getlatentic/haas/internal/sandbox"
	"github.com/getlatentic/haas/internal/setup"
	"github.com/getlatentic/haas/internal/signing"
	"github.com/getlatentic/haas/internal/web"
)

func serve(args []string) int {
	if len(args) != 0 {
		usage()
		return 2
	}
	password := os.Getenv("HAAS_PASSWORD")
	if password == "" {
		// A URL that spends money and runs code is not left open because a
		// variable was forgotten.
		fmt.Fprintln(os.Stderr, "haas: set HAAS_PASSWORD; the server will not start without a sign-in")
		return 2
	}
	if err := ready(); err != nil {
		return fail(err)
	}
	index, err := jobs.Open(setting("HAAS_JOBS", "jobs"))
	if err != nil {
		return fail(err)
	}
	// An organisation's front door writes what it may run, and this server reads it
	// as it goes. One that cannot read it does not start.
	if path := os.Getenv("HAAS_QUOTA"); path != "" {
		limits := quota.Open(path)
		if _, err := limits.Current(); err != nil {
			return fail(err)
		}
		index.UseQuota(limits)
	}
	// The directory keeps the name it was written under: what is in it is the
	// same thing, read now as an instance with threads rather than as one
	// conversation.
	instances, err := instance.Open(setting("HAAS_SESSIONS", "sessions"))
	if err != nil {
		return fail(err)
	}
	way, err := openWayOut(index.Root())
	if err != nil {
		return fail(err)
	}
	stopProxy, err := way.Listen()
	if err != nil {
		return fail(err)
	}
	defer stopProxy()
	index.SetLimits(jobs.Limits{Person: number("HAAS_LIMIT_PERSON", 0), Team: number("HAAS_LIMIT_TEAM", 0)})
	computers, err := computersFromEnv()
	if err != nil {
		return fail(err)
	}
	work, err := chosenWork(way, instances, index, computers)
	if err != nil {
		return fail(err)
	}
	var sharing *sharedApps
	if computers != nil {
		cfg, err := openshell.FromEnv()
		if err != nil {
			return fail(err)
		}
		shared, err := apps.Open(setting("HAAS_APPS", "apps"))
		if err != nil {
			return fail(err)
		}
		sharing = newSharedApps(cfg, computers, shared)
		tending, stopTending := context.WithCancel(context.Background())
		defer stopTending()
		go tendComputers(tending, cfg, computers, func(person string) bool { return running(index, person) || sharing.serving(person) },
			30*time.Second, time.Duration(number("HAAS_COMPUTER_IDLE_MINUTES", 30))*time.Minute)
	}
	queue := jobs.Start(index, work, number("HAAS_JOB_CONCURRENCY", 1))
	defer queue.Stop()

	// Asked once, at startup: the answer changes only when the image does, and
	// asking per page would start a container for every row somebody looks at.
	offered := surveyed(harness.Names())
	for _, entry := range offered {
		if !entry.Ready {
			fmt.Printf("  %-10s not offered: %s\n", entry.Name, entry.Why)
		}
	}
	// Reached at startup, so a wrong issuer fails here rather than at
	// somebody's first sign-in.
	identity, err := web.NewIdentity(context.Background(),
		os.Getenv("HAAS_OIDC_ISSUER"), os.Getenv("HAAS_OIDC_CLIENT_ID"),
		os.Getenv("HAAS_OIDC_CLIENT_SECRET"), os.Getenv("HAAS_OIDC_REDIRECT"),
		web.Access{
			Allowed:      values("HAAS_ALLOWED"),
			RunnerGroups: values("HAAS_RUNNER_GROUPS"),
			AdminGroups:  values("HAAS_ADMIN_GROUPS"),
			GroupsClaim:  os.Getenv("HAAS_OIDC_GROUPS_CLAIM"),
			Scopes:       values("HAAS_OIDC_SCOPES"),
		})
	if err != nil {
		return fail(err)
	}
	if identity.Configured() {
		fmt.Println("signing in with", os.Getenv("HAAS_OIDC_ISSUER"))
	}
	pages, err := web.New(index, instances, queue, password, identity, offered, offeredModels(way.credentials, values("HAAS_MODELS"), harness.Names()))
	if err != nil {
		return fail(err)
	}
	if sharing != nil {
		pages.UseApps(sharing.store, sharing)
	}
	pages.UseLedger(way.ledger)
	pages.UseHealth(web.Health{Release: releaseName(), DeployResult: os.Getenv("HAAS_DEPLOY_RESULT"), Proxy: way.ProxyCounts()})
	pages.UseAdmins(values("HAAS_ADMINS"))
	pages.UseLocalRepos(values("HAAS_LOCAL_REPOS"))
	// Setups are kept beside the instances. The MCP servers they choose from
	// are the operator's, one file each in HAAS_MCP.
	setups, err := setup.Open(setting("HAAS_SETUPS", "setups"))
	if err != nil {
		return fail(err)
	}
	servers, err := setup.OpenServers(os.Getenv("HAAS_MCP"))
	if err != nil {
		return fail(err)
	}
	pages.UseSetups(setups, servers)
	pages.UseInBuilding(way.InBuilding())
	lineups, err := compare.OpenBoard(setting("HAAS_COMPARISONS", "comparisons"))
	if err != nil {
		return fail(err)
	}
	pages.UseBoard(lineups)
	// The job page names a signer only as this keyring checked it, the way
	// verify, show, ship and export do.
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return fail(err)
	}
	pages.UseKeyring(keyring)
	// What changes on the platform is recorded beside the jobs, in a file only
	// this server appends to.
	changes, err := openChanges(keyring)
	if err != nil {
		return fail(err)
	}
	defer changes.Close()
	pages.UseChanges(changes)
	if path := os.Getenv("HAAS_API_TOKENS"); path != "" {
		tokens, err := apitoken.Open(path)
		if err != nil {
			return fail(err)
		}
		pages.UseTokens(tokens)
	}
	background, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()
	if dir := os.Getenv("HAAS_RETENTION"); dir != "" {
		policy, err := retention.LoadPolicy(dir)
		if err != nil {
			return fail(err)
		}
		pages.UseRetention(dir, policy)
		go pages.RunRetention(background, time.Duration(number("HAAS_RETENTION_MINUTES", 60))*time.Minute)
	}
	pages.UseSettings(governing())
	front, err := frontDoorFromEnv()
	if err != nil {
		return fail(err)
	}
	if front != nil {
		pages.UseFrontDoor(*front)
	}
	address := setting("HAAS_ADDR", "127.0.0.1:8040")
	server := &http.Server{
		Addr:              address,
		Handler:           accesslog.Keep(accesslog.Stderr())(pages.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		// A job's stream stays open for as long as the job runs, so there is no
		// write timeout to set here; nginx holds the same view.
	}
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopping
		context, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = server.Shutdown(context)
	}()
	fmt.Printf("haas is listening on %s, with %d job(s) at a time\n", address, number("HAAS_JOB_CONCURRENCY", 1))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	return 0
}

// chosenWork decides where a job runs. A container is the product; running a
// job in this process is for a laptop with no image built.
func chosenWork(way *WayOut, instances *instance.Store, index *jobs.Index, registry *computer.Registry) (jobs.Work, error) {
	switch os.Getenv("HAAS_SANDBOX") {
	case "here":
		return here(instances), nil
	case "openshell":
		cfg, err := openshell.FromEnv()
		if err != nil {
			return nil, err
		}
		return inSandbox(way, cfg, index, registry, instances), nil
	}
	return func(dir job.Dir) (int, error) {
		spec, err := job.Read(dir)
		if err != nil {
			return 0, err
		}
		// The checkout happens outside the box, so a repository credential
		// never goes in with the agent.
		if err := runner.Fill(spec, dir); err != nil {
			return 0, err
		}
		defer way.Done(dir)
		exit, err := way.Run(dir, spec, staying(instances, spec)...)
		// A turn learns what the harness calls the conversation, and the next
		// turn is handed it back. Without this every turn starts again.
		if manifest, read := evidence.Read(dir); read == nil {
			learnConversation(instances, spec, manifest.Session)
		}
		return exit, err
	}, nil
}

// inSandbox runs each job's harness in an OpenShell sandbox. The checkout
// happens here, outside the sandbox, as it does for a container.
func inSandbox(way *WayOut, cfg openshell.Config, index *jobs.Index, registry *computer.Registry, instances *instance.Store) jobs.Work {
	return func(dir job.Dir) (int, error) {
		spec, err := job.Read(dir)
		if err != nil {
			return 0, err
		}
		if err := runner.Fill(spec, dir); err != nil {
			return 0, err
		}
		defer way.Done(dir)
		if err := beforeTurn(instances, spec, dir); err != nil {
			return 0, err
		}
		exit, err := ranInSandbox(way, cfg, index, registry, dir, spec)
		if afterErr := afterTurn(instances, spec, dir); afterErr != nil && err == nil {
			err = afterErr
		}
		return exit, err
	}
}

func setting(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// values reads a setting written as one comma-separated line, which is how a
// systemd unit carries a list. Blank members are dropped, so a trailing comma
// does not become one.
func values(name string) []string {
	var members []string
	for _, each := range strings.Split(os.Getenv(name), ",") {
		if trimmed := strings.TrimSpace(each); trimmed != "" {
			members = append(members, trimmed)
		}
	}
	return members
}

func number(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

// here runs a job in this process, which is what a laptop with no image built
// has. A turn in a container gets the instance's workspace mounted over its
// own; without one, the job's workspace is made a link to it instead, so a
// conversation works the same way in both places.
func here(instances *instance.Store) jobs.Work {
	return func(dir job.Dir) (int, error) {
		spec, err := job.Read(dir)
		if err != nil {
			return 0, err
		}
		if spec.Session != "" {
			if err := pointAt(instances.Workspace(spec.Session), dir); err != nil {
				return 0, err
			}
		}
		return runner.Inside(dir)
	}
}

// offeredModels is what the pages may offer, by harness. A credential can say
// which harnesses it serves, because a provider can refuse what one harness
// sends while answering another; a credential that says nothing serves them
// all, which is what a deployment has until it finds out otherwise.
func credentialOf(model string) string {
	named, _ := proxy.Named(model)
	return named
}

func offeredModels(credentials proxy.Credentials, models, harnesses []string) web.Models {
	if len(credentials) == 0 {
		return web.Everything(models)
	}
	offered := web.Models{"": models}
	for _, name := range harnesses {
		// An entry of its own, empty when no model suits it, so that a harness nothing
		// serves is not offered every model as one with no entry is.
		offered[name] = []string{}
		speaks := "openai"
		if adapter, err := harness.Load("", name); err == nil && adapter.Speaks != "" {
			speaks = adapter.Speaks
		}
		for _, model := range models {
			credential, held := credentials.Get(credentialOf(model))
			// A harness is offered a model only through a credential that
			// answers in the shape it calls: Claude Code sent to an
			// OpenAI-shaped endpoint is told its model does not exist.
			if !held || (credential.Serves(name) && credential.Speaks() == speaks) {
				offered[name] = append(offered[name], model)
			}
		}
	}
	return offered
}

// pointAt makes a job's workspace a link to the instance's.
//
// The job directory is made before it is queued, which leaves a real, empty
// directory where the workspace goes; a link cannot be put over one. Remove
// refuses a directory with anything in it, so a job that has its own checkout
// is never quietly replaced.
func pointAt(workspace string, dir job.Dir) error {
	if err := dir.Make(); err != nil {
		return err
	}
	if err := os.Remove(dir.Workspace()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(workspace, dir.Workspace())
}

// staying is what a turn needs that outlives its container: the conversation's
// working directory, and the memory the harness keeps of the conversation.
// A harness stores that memory in its own data directory rather than beside the
// files it edits, which is why there are two.
func staying(instances *instance.Store, spec job.Spec) []sandbox.Mount {
	if spec.Session == "" {
		return nil
	}
	return []sandbox.Mount{
		{From: instances.Workspace(spec.Session), To: "/job/workspace"},
		{From: instances.State(spec.Session), To: "/root/.local/share"},
	}
}

// surveyed is what this deployment's image carries. In a container that is asked
// of the runtime, and in a sandbox on a gateway it is asked of a sandbox.
func surveyed(names []string) []catalog.Entry {
	if os.Getenv("HAAS_SANDBOX") == "openshell" {
		if cfg, err := openshell.FromEnv(); err == nil {
			return catalog.SurveyOf(openshell.Image(cfg), names)
		}
	}
	return catalog.Survey(sandbox.Runtime(), sandbox.Image(), harness.Names())
}

// ranInSandbox runs a job in the computer of the person who asked for it, and one asked
// for by a script or a token, which is nobody's, in a sandbox of its own.
func ranInSandbox(way *WayOut, cfg openshell.Config, index *jobs.Index, registry *computer.Registry, dir job.Dir, spec job.Spec) (int, error) {
	if person := personWhoAsked(index, dir); registry != nil && person != "" {
		return way.RunInComputer(dir, spec, cfg, registry, person)
	}
	return way.RunInSandbox(dir, spec, cfg)
}
