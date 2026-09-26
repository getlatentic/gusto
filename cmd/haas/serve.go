package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/runner"
	"github.com/getlatentic/haas/internal/sandbox"
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
	queue := jobs.Start(index, chosenWork(), number("HAAS_JOB_CONCURRENCY", 1))
	defer queue.Stop()

	pages, err := web.New(index, queue, password, harness.Names())
	if err != nil {
		return fail(err)
	}
	address := setting("HAAS_ADDR", "127.0.0.1:8040")
	server := &http.Server{
		Addr:              address,
		Handler:           pages.Handler(),
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
func chosenWork() jobs.Work {
	if os.Getenv("HAAS_SANDBOX") == "here" {
		return runner.Inside
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
		return sandbox.Run(dir)
	}
}

func setting(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func number(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}
