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
	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/runner"
	"github.com/getlatentic/haas/internal/sandbox"
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
	index, err := jobs.Open(setting("HAAS_JOBS", "jobs"))
	if err != nil {
		return fail(err)
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
	queue := jobs.Start(index, chosenWork(way, instances), number("HAAS_JOB_CONCURRENCY", 1))
	defer queue.Stop()

	// Asked once, at startup: the answer changes only when the image does, and
	// asking per page would start a container for every row somebody looks at.
	offered := catalog.Survey(sandbox.Runtime(), sandbox.Image(), harness.Names())
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
		values("HAAS_ALLOWED"))
	if err != nil {
		return fail(err)
	}
	if identity.Configured() {
		fmt.Println("signing in with", os.Getenv("HAAS_OIDC_ISSUER"))
	}
	pages, err := web.New(index, instances, queue, password, identity, offered, values("HAAS_MODELS"))
	if err != nil {
		return fail(err)
	}
	pages.UseLedger(way.ledger)
	// The job page names a signer only as this keyring checked it, the way
	// verify, show, ship and export do.
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return fail(err)
	}
	pages.UseKeyring(keyring)
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
func chosenWork(way *WayOut, instances *instance.Store) jobs.Work {
	if os.Getenv("HAAS_SANDBOX") == "here" {
		return here(instances)
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
		if spec.Session != "" && spec.Thread != "" {
			if manifest, read := evidence.Read(dir); read == nil {
				_ = instances.Said(spec.Session, spec.Thread, "", manifest.Session)
			}
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
