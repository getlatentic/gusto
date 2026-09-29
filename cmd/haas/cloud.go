package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/getlatentic/haas/internal/accesslog"
	"github.com/getlatentic/haas/internal/cloud"
)

// cloudCommand runs the front door of a hosted haas: sign-up, sign-in and the
// organisations' records. It runs no jobs; each organisation has a server of its
// own.
func cloudCommand(args []string) int {
	front, err := openCloud()
	if err != nil {
		return fail(err)
	}
	address := setting("HAAS_CLOUD_ADDR", "127.0.0.1:8050")
	server := &http.Server{
		Addr:              address,
		Handler:           accesslog.Keep(accesslog.Stderr())(front.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	stopping := make(chan os.Signal, 1)
	signal.Notify(stopping, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stopping
		context, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = server.Shutdown(context)
	}()
	fmt.Printf("the front door is listening on %s\n", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	front.Wait()
	return 0
}

// openCloud reads the front door's settings and opens what it keeps. Links
// that expired are swept when it starts and every hour after.
func openCloud() (*cloud.Server, error) {
	address := setting("HAAS_CLOUD_ADDR", "127.0.0.1:8050")
	public, err := url.Parse(setting("HAAS_CLOUD_URL", "http://"+address))
	if err != nil || (public.Scheme != "http" && public.Scheme != "https") || public.Host == "" {
		return nil, errors.New("HAAS_CLOUD_URL is the address links in mail begin with, as https://cloud.example.com")
	}
	root := setting("HAAS_CLOUD_ROOT", "cloud")
	limits := cloud.DefaultLimits()
	limits.Trial.Jobs = number("HAAS_CLOUD_TRIAL_JOBS", limits.Trial.Jobs)
	limits.Trial.Concurrent = number("HAAS_CLOUD_TRIAL_CONCURRENT", limits.Trial.Concurrent)
	limits.Verified.Jobs = number("HAAS_CLOUD_JOBS", limits.Verified.Jobs)
	limits.Verified.Concurrent = number("HAAS_CLOUD_CONCURRENT", limits.Verified.Concurrent)
	limits.Verified.Members = number("HAAS_CLOUD_MEMBERS", limits.Verified.Members)
	limits.Verified.Computers = number("HAAS_CLOUD_COMPUTERS", limits.Verified.Computers)
	store, err := cloud.Open(root, cloud.Settings{
		AllowFreeMail:  os.Getenv("HAAS_CLOUD_FREE_MAIL") == "allow",
		RefusedDomains: lowered(values("HAAS_CLOUD_REFUSED_DOMAINS")),
		PerDomain:      number("HAAS_CLOUD_PER_DOMAIN", 3),
		Limits:         limits,
	})
	if err != nil {
		return nil, err
	}
	sessions, err := cloud.OpenSessions(root, public.Scheme == "https")
	if err != nil {
		return nil, err
	}
	mail, err := cloud.MailFrom(os.Getenv("HAAS_CLOUD_MAIL"), filepath.Join(root, "outbox"))
	if err != nil {
		return nil, err
	}
	front, err := cloud.New(cloud.Config{
		Store: store, Mail: mail, Sessions: sessions, URL: public,
		Domain:     setting("HAAS_CLOUD_DOMAIN", "localhost"),
		TrustProxy: os.Getenv("HAAS_CLOUD_TRUST_PROXY") == "1",
		PerAddress: number("HAAS_CLOUD_PER_ADDRESS", 0), PerMailbox: number("HAAS_CLOUD_PER_MAILBOX", 0), InAll: number("HAAS_CLOUD_IN_ALL", 0),
	})
	if err != nil {
		return nil, err
	}
	go sweepLinks(store)
	return front, nil
}

func sweepLinks(store *cloud.Store) {
	for {
		if _, err := store.Sweep(); err != nil {
			fmt.Fprintf(os.Stderr, "the front door could not sweep expired links: %v\n", err)
		}
		time.Sleep(time.Hour)
	}
}

func lowered(members []string) []string {
	out := make([]string, len(members))
	for i, member := range members {
		out[i] = strings.ToLower(member)
	}
	return out
}
