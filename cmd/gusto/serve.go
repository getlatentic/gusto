package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/getlatentic/gusto/internal/accesslog"
	"github.com/getlatentic/gusto/internal/agentinstall"
	"github.com/getlatentic/gusto/internal/apitoken"
	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/computer"
	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/integration"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/notify"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/orgsettings"
	"github.com/getlatentic/gusto/internal/preset"
	"github.com/getlatentic/gusto/internal/quota"
	"github.com/getlatentic/gusto/internal/retention"
	"github.com/getlatentic/gusto/internal/roomlive"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/internal/web"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/pkg/spend"
	"github.com/getlatentic/gusto/pkg/wire"
)

func serve(args []string) int {
	if len(args) != 0 {
		return usageError("serve", "takes no arguments")
	}
	password := os.Getenv("GUSTO_PASSWORD")
	if password == "" {
		// A URL that spends money and runs code is not left open because a
		// variable was forgotten.
		fmt.Fprintln(os.Stderr, "gusto: set GUSTO_PASSWORD; the server will not start without a sign-in")
		return 2
	}
	accesslog.TrustForwarded(os.Getenv("GUSTO_BEHIND_PROXY") == "1")
	local := os.Getenv("GUSTO_LOCAL") == "1"
	if local && !onLoopback(setting("GUSTO_ADDR", "127.0.0.1:8040")) {
		fmt.Fprintln(os.Stderr, "gusto: a server for one person on their own computer listens on the loopback only, and GUSTO_ADDR names another address")
		return 2
	}
	if err := ready(); err != nil {
		return fail(err)
	}
	index, err := jobs.Open(setting("GUSTO_JOBS", "jobs"))
	if err != nil {
		return fail(err)
	}
	// An organization's gusto cloud writes what it may run, and this server reads it
	// as it goes. One that cannot read it does not start.
	var limits *quota.File
	if path := os.Getenv("GUSTO_QUOTA"); path != "" {
		limits = quota.Open(path)
		if _, err := limits.Current(); err != nil {
			return fail(err)
		}
		index.UseQuota(limits)
	}
	// The directory keeps the name it was written under: what is in it is the
	// same thing, read now as an instance with threads rather than as one
	// conversation.
	instances, err := instance.Open(setting("GUSTO_SESSIONS", "sessions"))
	if err != nil {
		return fail(err)
	}
	way, err := openWayOut(index.Root())
	if err != nil {
		return fail(err)
	}
	if limits != nil {
		if err := way.ledger.UseCredit(creditOf(limits), filepath.Join(filepath.Dir(os.Getenv("GUSTO_QUOTA")), "credit-used.json")); err != nil {
			return fail(err)
		}
	}
	stopProxy, err := way.Listen()
	if err != nil {
		return fail(err)
	}
	defer stopProxy()
	index.SetLimits(jobs.Limits{Person: number("GUSTO_LIMIT_PERSON", 0), Team: number("GUSTO_LIMIT_TEAM", 0)})
	computers, err := computersFromEnv()
	if err != nil {
		return fail(err)
	}
	rooms := newRoomRuns(instances, func(dir job.Dir) string { return personWhoAsked(index, dir) })
	way.UseRunRecords(func(dir job.Dir, spec job.Spec) evidence.Run { return runRecord(index, instances, dir, spec) })
	way.UseAskers(func(id string) string {
		if record, found := index.Get(id); found {
			return record.Asked
		}
		return ""
	})
	work, err := chosenWork(way, instances, index, computers, rooms)
	if err != nil {
		return fail(err)
	}
	var sharing *sharedApps
	if computers != nil {
		cfg, err := openshell.FromEnv()
		if err != nil {
			return fail(err)
		}
		shared, err := apps.Open(setting("GUSTO_APPS", "apps"))
		if err != nil {
			return fail(err)
		}
		sharing = newSharedApps(cfg, computers, shared)
		tending, stopTending := context.WithCancel(context.Background())
		defer stopTending()
		go tendComputers(tending, cfg, computers, func(person string) bool { return running(index, person) || sharing.serving(person) },
			30*time.Second, time.Duration(number("GUSTO_COMPUTER_IDLE_MINUTES", 30))*time.Minute)
		if way.spares, err = openSpareComputers(tending, cfg, os.Getenv("GUSTO_ORG"), "http://"+cfg.Host+":"+way.proxyPort()+"/model/v1"); err != nil {
			return fail(err)
		}
		defer way.spares.close()
	}
	queue := jobs.StartStoppable(index, work, number("GUSTO_JOB_CONCURRENCY", 1))
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
		os.Getenv("GUSTO_OIDC_ISSUER"), os.Getenv("GUSTO_OIDC_CLIENT_ID"),
		os.Getenv("GUSTO_OIDC_CLIENT_SECRET"), os.Getenv("GUSTO_OIDC_REDIRECT"),
		web.Access{
			Allowed:      values("GUSTO_ALLOWED"),
			RunnerGroups: values("GUSTO_RUNNER_GROUPS"),
			AdminGroups:  values("GUSTO_ADMIN_GROUPS"),
			GroupsClaim:  os.Getenv("GUSTO_OIDC_GROUPS_CLAIM"),
			Scopes:       values("GUSTO_OIDC_SCOPES"),
		})
	if err != nil {
		return fail(err)
	}
	if identity.Configured() {
		fmt.Println("signing in with", os.Getenv("GUSTO_OIDC_ISSUER"))
	}
	pages, err := web.New(index, instances, queue, password, identity, offered, web.Models(values("GUSTO_MODELS")))
	if err != nil {
		return fail(err)
	}
	pages.UseRoomHold(rooms.hub.Hold)
	if wall != nil {
		pages.UseWall(theWall())
	}
	if local {
		pages.UseFoundKeys(values("GUSTO_FOUND_KEYS"))
		pages.UseNeeds(func() []string {
			adoptRecordedPath()
			return localNeeds()
		})
	}
	// On a person's own computer gusto installs the agents it does not find into a folder of its own, which it looks in first.
	var installer *agentinstall.Installer
	if local && runsHere() {
		dir, err := filepath.Abs(setting("GUSTO_AGENTS", "agents"))
		if err != nil {
			return fail(err)
		}
		installer = &agentinstall.Installer{Dir: dir}
		os.Setenv("PATH", installer.WithBin(os.Getenv("PATH")))
		pages.UseInstaller(installer)
	}
	if runsHere() {
		// A person installs an agent on their own computer while the server runs, and looking at the PATH costs nothing.
		pages.UseLiveCatalog(func() []catalog.Entry {
			if local {
				adoptRecordedPath()
			}
			if installer != nil {
				os.Setenv("PATH", installer.WithBin(os.Getenv("PATH")))
			}
			return surveyed(harness.Names())
		})
	}
	if sharing != nil {
		pages.UseApps(sharing.store, sharing)
	}
	if computers != nil {
		pages.UseComputers(computers)
	}
	if local {
		pages.UseLocalSignIn(password, localPerson())
	}
	if path := os.Getenv("GUSTO_ORG_SETTINGS"); path != "" {
		pages.UseOrgSettings(path)
		settings := orgsettings.Open(path)
		// Settings that cannot be read leave runs bound, the safer way.
		way.UseRunTokens(func() bool {
			chosen, err := settings.Read()
			return err == nil && chosen.PlainRunTokens
		})
	}
	if harness, model, found := strings.Cut(os.Getenv("GUSTO_CHAT_DEFAULT"), "="); found {
		pages.UseChatDefault(strings.TrimSpace(harness), strings.TrimSpace(model))
	}
	reg, offeredNow, err := openRegistry(way, func() { pages.UseInBuilding(way.InBuilding()) })
	if err != nil {
		return fail(err)
	}
	if reg != nil {
		pages.UseRegistry(reg, offeredNow)
	}
	pages.UseLedger(way.ledger)
	if way.proxy != nil {
		pages.UseReach(func(model string, shape wire.Shape) (bool, bool) {
			return way.proxy.Reaches(context.Background(), model, shape)
		})
	}
	liveCtx, stopLive := context.WithCancel(context.Background())
	defer stopLive()
	if _, err := startLive(liveCtx, way, index, pages, reg != nil); err != nil {
		return fail(err)
	}
	pages.UseHealth(web.Health{Release: releaseName(), DeployResult: os.Getenv("GUSTO_DEPLOY_RESULT"), Proxy: way.ProxyCounts()})
	pages.UseAdmins(values("GUSTO_ADMINS"))
	pages.UseLocalRepos(values("GUSTO_LOCAL_REPOS"))
	// Setups are kept beside the instances. The MCP servers they choose from
	// are the operator's, one file each in GUSTO_MCP.
	setups, err := setup.Open(setting("GUSTO_SETUPS", "setups"))
	if err != nil {
		return fail(err)
	}
	servers, err := setup.OpenServers(os.Getenv("GUSTO_MCP"))
	if err != nil {
		return fail(err)
	}
	// Integrations are plugins an administrator imported. A setup names them, and a task gets the version that is on.
	integrations, err := integration.Open(setting("GUSTO_INTEGRATIONS", "integrations"))
	if err != nil {
		return fail(err)
	}
	setups.UseIntegrations(integrations)
	pages.UseIntegrations(integrations, integration.Clone{AllowPrivate: os.Getenv("GUSTO_INTEGRATIONS_PRIVATE") == "1"})
	pages.UseSetups(setups, servers)
	presets, err := preset.Open(setting("GUSTO_PRESETS", "presets"))
	if err != nil {
		return fail(err)
	}
	pages.UsePresets(presets)
	agents, err := crew.Open(setting("GUSTO_CREW", "crew"))
	if err != nil {
		return fail(err)
	}
	pages.UseCrew(agents)
	pages.UseDesktop(os.Getenv("GUSTO_DESKTOP"))
	// What signs people in is kept beside the sessions, so that a deploy does not sign them out.
	if err := pages.UseSessionFile(setting("GUSTO_KEEPER", filepath.Join(setting("GUSTO_SESSIONS", "sessions"), ".keeper.json"))); err != nil {
		return fail(err)
	}
	pages.UseInBuilding(way.InBuilding())
	lineups, err := compare.OpenBoard(setting("GUSTO_COMPARISONS", "comparisons"))
	if err != nil {
		return fail(err)
	}
	pages.UseBoard(lineups)
	// The task page names a signer only as this keyring checked it, the way
	// verify, show, pr open and export do.
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return fail(err)
	}
	pages.UseKeyring(keyring)
	// What changes on the platform is recorded beside the tasks, in a file only
	// this server appends to.
	changes, err := openChanges(keyring)
	if err != nil {
		return fail(err)
	}
	defer changes.Close()
	pages.UseChanges(changes)
	if path := os.Getenv("GUSTO_API_TOKENS"); path != "" {
		tokens, err := apitoken.Open(path)
		if err != nil {
			return fail(err)
		}
		pages.UseTokens(tokens)
	}
	stopUsage, err := startUsage(way.proxy, pages, index.Root())
	if err != nil {
		return fail(err)
	}
	defer stopUsage()
	stopTasks, err := startTasks(index, pages, queue.Stop)
	if err != nil {
		return fail(err)
	}
	defer stopTasks()
	background, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()
	news, err := routineNews()
	if err != nil {
		return fail(err)
	}
	pages.UseRoutineNews(news, os.Getenv("GUSTO_PUBLIC_URL"))
	go pages.RunRoutines(background, time.Minute)
	if dir := os.Getenv("GUSTO_RETENTION"); dir != "" {
		policy, err := retention.LoadPolicy(dir)
		if err != nil {
			return fail(err)
		}
		pages.UseRetention(dir, policy)
		go pages.RunRetention(background, time.Duration(number("GUSTO_RETENTION_MINUTES", 60))*time.Minute)
	}
	if path := os.Getenv("GUSTO_MEMBERS"); path != "" {
		pages.UseRoster(path)
	}
	if dir := os.Getenv("GUSTO_INBOX"); dir != "" {
		pages.UseInbox(dir)
		go pages.RunInbox(background, 2*time.Second)
	}
	pages.UseSettings(governing())
	// A room's page follows the room over a WebSocket, and falls back to asking every few seconds when it has none.
	if err := pages.UseLive(roomlive.Options{MaxConnections: number("GUSTO_LIVE_CONNECTIONS", 10000)}); err != nil {
		return fail(err)
	}
	front, err := frontDoorFromEnv()
	if err != nil {
		return fail(err)
	}
	if front != nil {
		pages.UseFrontDoor(*front)
	}
	address := setting("GUSTO_ADDR", "127.0.0.1:8040")
	server := &http.Server{
		Addr:              address,
		Handler:           accesslog.Keep(accesslog.Stderr())(pages.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		// A task's stream stays open for as long as the task runs, so there is no
		// write timeout to set here; nginx holds the same view.
	}
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopping
		context, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		// A hijacked connection is not the HTTP server's to wait for, so the sockets are closed, and waited for, first.
		if err := pages.CloseLive(context); err != nil {
			fmt.Fprintln(os.Stderr, "gusto: the live connections did not all close in time:", err)
		}
		_ = server.Shutdown(context)
	}()
	fmt.Printf("gusto is listening on %s, with %d task(s) at a time\n", address, number("GUSTO_JOB_CONCURRENCY", 1))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	return 0
}

// chosenWork decides where a task runs. A container is the product; running a
// task in this process is for a laptop with no image built.
func chosenWork(way *WayOut, instances *instance.Store, index *jobs.Index, registry *computer.Registry, rooms *roomRuns) (jobs.StoppableWork, error) {
	switch os.Getenv("GUSTO_SANDBOX") {
	case "here", "process":
		return here(way, instances, rooms), nil
	case "openshell":
		if err := openshell.HostCannotMakeVMs(); err != nil {
			return nil, err
		}
		cfg, err := openshell.FromEnv()
		if err != nil {
			return nil, err
		}
		return inSandbox(way, cfg, index, registry, instances, rooms), nil
	}
	return func(ctx context.Context, dir job.Dir) (int, error) {
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
		exit, err := rooms.around(ctx, spec, dir, func() (int, error) {
			return way.RunContext(ctx, dir, spec, ownerOf(index, dir, spec), staying(instances, spec)...)
		})
		// A turn learns what the agent calls the conversation, and the next
		// turn is handed it back. Without this every turn starts again.
		if manifest, read := evidence.Read(dir); read == nil {
			learnConversation(instances, spec, manifest.Session)
		}
		return exit, err
	}, nil
}

// inSandbox runs each task's agent in an OpenShell sandbox. The checkout
// happens here, outside the sandbox, as it does for a container.
func inSandbox(way *WayOut, cfg openshell.Config, index *jobs.Index, registry *computer.Registry, instances *instance.Store, rooms *roomRuns) jobs.StoppableWork {
	return func(ctx context.Context, dir job.Dir) (int, error) {
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
		return rooms.around(ctx, spec, dir, func() (int, error) {
			exit, err := ranInSandbox(ctx, way, cfg, index, registry, dir, spec)
			if afterErr := afterTurn(instances, spec, dir); afterErr != nil && err == nil {
				err = afterErr
			}
			return exit, err
		})
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

// pointAt makes a task's workspace a link to the instance's.
//
// The task directory is made before it is queued, which leaves a real, empty
// directory where the workspace goes; a link cannot be put over one. Remove
// refuses a directory with anything in it, so a task that has its own checkout
// is never quietly replaced.
func pointAt(workspace string, dir job.Dir) error {
	if err := dir.Make(); err != nil {
		return err
	}
	if err := os.Remove(dir.Workspace()); err != nil && !os.IsNotExist(err) {
		return err
	}
	// A link's relative target is read from the link's own folder, not from where the server runs.
	target, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	return linkDir(target, dir.Workspace())
}

// surveyed is the agents this deployment can run. In a container that is asked
// of the runtime, in a sandbox on a gateway it is asked of a sandbox, and a task
// run in this process finds its program on this computer's PATH.
func surveyed(names []string) []catalog.Entry {
	switch os.Getenv("GUSTO_SANDBOX") {
	case "here", "process":
		return catalog.SurveyOf(catalog.Machine(), names)
	case "openshell":
		if cfg, err := openshell.FromEnv(); err == nil {
			return catalog.SurveyOf(openshell.Remembered(openshell.Image(cfg), cfg.Image), names)
		}
	}
	return catalog.Survey(sandbox.Runtime(), sandbox.Image(), harness.Names())
}

// ranInSandbox runs a task in the computer of the person who asked for it, and one asked
// for by a script or a token, which is nobody's, in a sandbox of its own.
func ranInSandbox(ctx context.Context, way *WayOut, cfg openshell.Config, index *jobs.Index, registry *computer.Registry, dir job.Dir, spec job.Spec) (int, error) {
	if person := personWhoPays(index, dir); registry != nil && person != "" {
		return way.RunInComputer(ctx, dir, spec, cfg, registry, person)
	}
	return way.RunInSandboxContext(ctx, dir, spec, cfg)
}

// routineNews is who is told how each run of a routine ended: an address that takes a signed request, named by
// GUSTO_NOTIFY_WEBHOOK and GUSTO_NOTIFY_SECRET, and mail to the person who added the routine, set up as GUSTO_CLOUD_MAIL is by
// GUSTO_MAIL and GUSTO_MAIL_TOKEN. With neither set nothing is sent, and the tray still hears of the turn.
func routineNews() (notify.Every, error) {
	var news notify.Every
	if url := os.Getenv("GUSTO_NOTIFY_WEBHOOK"); url != "" {
		hook := notify.Webhook{URL: url, Secret: os.Getenv("GUSTO_NOTIFY_SECRET")}
		if err := hook.Check(); err != nil {
			return nil, err
		}
		news = append(news, hook)
	}
	if how := os.Getenv("GUSTO_MAIL"); how != "" {
		sender, err := cloud.MailFrom(how, setting("GUSTO_MAIL_OUTBOX", "outbox"), os.Getenv("GUSTO_MAIL_TOKEN"))
		if err != nil {
			return nil, err
		}
		news = append(news, notify.Mail{Via: sender})
	}
	return news, nil
}

// creditOf is the credit an organization's gusto cloud gives it, read from its quota as the quota changes. A quota that cannot be
// read is a credit of nothing, so a server that lost its limits spends nothing of the operator's.
func creditOf(limits *quota.File) spend.CreditLimit {
	return func() (float64, bool) {
		current, err := limits.Current()
		if err != nil {
			return 0, true
		}
		return current.CreditUSD, current.CreditUSD > 0
	}
}
