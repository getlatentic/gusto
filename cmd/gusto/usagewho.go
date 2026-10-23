package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/web"
	"github.com/getlatentic/gusto/pkg/harness"
)

// attribution is who made a task's calls and who answered, for a command that runs with no server. It reads the
// tasks' records and the agents' files directly, since opening them as a server does would end any task a running
// server has in flight.
type attribution struct {
	tasks     string
	instances *instance.Store
	engines   []catalog.Entry
	provider  usage.ProviderFunc
}

func openAttribution(tasks string) (*attribution, error) {
	instances, err := instance.Open(setting("GUSTO_SESSIONS", "sessions"))
	if err != nil {
		return nil, err
	}
	engines := catalog.SurveyOf(catalog.Machine(), harness.Names())
	provider, err := providerOf(engines)
	if err != nil {
		return nil, err
	}
	return &attribution{tasks: tasks, instances: instances, engines: engines, provider: provider}, nil
}

// providerOf names providers as the server does: through the web server's own method, over the organization's
// registry when GUSTO_REGISTRY is set.
func providerOf(engines []catalog.Entry) (usage.ProviderFunc, error) {
	identity, err := web.NewIdentity(context.Background(), "", "", "", "", web.Access{})
	if err != nil {
		return nil, err
	}
	pages, err := web.New(nil, nil, nil, "unused", identity, engines, nil)
	if err != nil {
		return nil, err
	}
	if setting("GUSTO_REGISTRY", "") == "" {
		return pages.UsageProvider, nil
	}
	way, err := openWayOut(setting("GUSTO_JOBS", "jobs"))
	if err != nil {
		return nil, err
	}
	reg, offered, err := openRegistry(way, func() {})
	if err != nil {
		return nil, err
	}
	if reg != nil {
		pages.UseRegistry(reg, offered)
	}
	return pages.UsageProvider, nil
}

func (a *attribution) engineName(name string) string {
	for _, entry := range a.engines {
		if entry.Name == name && entry.Title != "" {
			return entry.Title
		}
	}
	return name
}

func (a *attribution) agentName(instanceID string) (string, bool) {
	held, found := a.instances.Get(instanceID)
	if !found {
		return "", false
	}
	template, _ := industry.Get(held.Setup)
	return usage.AgentName(held.Title, template.Title, a.engineName(held.Harness)), true
}

// who is usage.WhoOfTask over a record read from the task's directory.
func (a *attribution) who(task string) (usage.Who, bool) {
	text, err := os.ReadFile(filepath.Join(a.tasks, task, "state.json"))
	if err != nil {
		return usage.Who{}, false
	}
	var record jobs.Record
	if json.Unmarshal(text, &record) != nil {
		return usage.Who{}, false
	}
	return usage.WhoOfTask(record, a.engineName, a.agentName), true
}
