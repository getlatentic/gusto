package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/live"
	"github.com/getlatentic/haas/internal/spend"
	"github.com/getlatentic/haas/internal/web"
)

// liveSettings is what a server reads again while it runs, so that a change takes effect within a minute and nobody mid-task is
// sent away for a restart. HAAS_LIVE holds the limits and the models, HAAS_SPEND_POLICY the prices and caps, HAAS_TOOL_POLICY the
// tool rules, and HAAS_CREDENTIALS the credentials. A job already running keeps the credential it began with and asks the gateway's
// policy at each call. HAAS_RELOAD_SECONDS is how often they are looked at, twenty by default, and a change is taken when it is the same at two looks, so it shows within a minute.
//
// When the organisation has a registry its administrators change the models, the credentials and the prices from the console, and
// those are theirs: the files here change what the registry does not hold.
// pageHooks is what the pages are told when a setting is read again.
type pageHooks interface {
	UseLiveModels(func() web.Models)
	UseInBuilding(bool)
	NoteSetting(setting, was, now, refused string)
}

type liveSettings struct {
	way     *WayOut
	index   *jobs.Index
	pages   pageHooks
	managed bool
	// models is the list of models people are offered, as the settings file last gave it or the environment first did.
	models  atomic.Pointer[[]string]
	offered atomic.Pointer[web.Models]
}

// startLive reads the settings file as the server starts and watches the others until the context ends.
func startLive(ctx context.Context, way *WayOut, index *jobs.Index, pages pageHooks, managed bool) (*liveSettings, error) {
	l := &liveSettings{way: way, index: index, pages: pages, managed: managed}
	initial := values("HAAS_MODELS")
	l.models.Store(&initial)
	if err := l.applyFile(); err != nil {
		return nil, err
	}
	l.refresh()
	if !managed {
		pages.UseLiveModels(func() web.Models { return *l.offered.Load() })
	}
	every := time.Duration(number("HAAS_RELOAD_SECONDS", 20)) * time.Second
	go live.Watch(ctx, every, l.sources(), l.told)
	return l, nil
}

func (l *liveSettings) sources() []live.Source {
	sources := []live.Source{
		{Name: "HAAS_LIVE", Path: os.Getenv("HAAS_LIVE"), Apply: l.applyFile},
		{Name: "HAAS_SPEND_POLICY", Path: os.Getenv("HAAS_SPEND_POLICY"), Apply: l.applySpend},
	}
	if l.way.gateways != nil {
		sources = append(sources, live.Source{Name: "HAAS_TOOL_POLICY", Path: os.Getenv("HAAS_TOOL_POLICY"), Apply: l.way.gateways.reload})
	}
	if !l.managed && l.way.proxy != nil {
		sources = append(sources, live.Source{Name: "HAAS_CREDENTIALS", Path: os.Getenv("HAAS_CREDENTIALS"), Apply: l.applyCredentials})
	}
	return sources
}

// applyFile reads HAAS_LIVE: the limits, which the queue takes from the next job it picks, and the models.
func (l *liveSettings) applyFile() error {
	settings, err := live.ReadSettings(os.Getenv("HAAS_LIVE"))
	if err != nil {
		return err
	}
	person, team := number("HAAS_LIMIT_PERSON", 0), number("HAAS_LIMIT_TEAM", 0)
	if settings.LimitPerson != nil {
		person = *settings.LimitPerson
	}
	if settings.LimitTeam != nil {
		team = *settings.LimitTeam
	}
	l.index.SetLimits(jobs.Limits{Person: person, Team: team})
	models := values("HAAS_MODELS")
	if len(settings.Models) > 0 {
		models = settings.Models
	}
	l.models.Store(&models)
	l.refresh()
	return nil
}

// applySpend reads the prices and the caps again. Counted spend is kept. When the registry holds the prices, the file's are not used.
func (l *liveSettings) applySpend() error {
	policy, err := spend.LoadPolicy(os.Getenv("HAAS_SPEND_POLICY"))
	if err != nil {
		return err
	}
	if l.managed {
		policy.Prices = l.way.ledger.Policy().Prices
	}
	l.way.ledger.SetPolicy(policy)
	return nil
}

func (l *liveSettings) applyCredentials() error {
	credentials, err := heldCredentials()
	if err != nil {
		return err
	}
	if len(credentials) == 0 {
		return fmt.Errorf("HAAS_CREDENTIALS holds no credential, and a server that has a proxy needs one: what it holds is kept")
	}
	l.way.SetCredentials(credentials)
	l.refresh()
	return nil
}

// refresh works out the models offered now: the list, through the credentials held, for each harness.
func (l *liveSettings) refresh() {
	offered := offeredModels(l.way.credentials(), *l.models.Load(), harness.Names())
	l.offered.Store(&offered)
	l.pages.UseInBuilding(l.way.InBuilding())
}

// told writes a change to the change record, and says a refusal on the server's own log.
func (l *liveSettings) told(change live.Change) {
	refused := ""
	if change.Err != nil {
		refused = change.Err.Error()
		fmt.Fprintf(os.Stderr, "%s changed and was not taken, so it stays as it was: %v\n", change.Name, change.Err)
	}
	l.pages.NoteSetting(change.Name, change.Was, change.Now, refused)
}
