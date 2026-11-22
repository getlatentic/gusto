package operator

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/web"
)

// page is the operator's page as the web app's cloud entry draws it, with every time already in the words it is shown in.
type page struct {
	Page, Title, Notice string
	Paused, LimitsSet   bool
	Limits              cloud.DemoLimits
	Running             []runningRow
	Leads               []leadRow
	Contacts            []contactRow
}

type runningRow struct {
	Name, Company, State, Left string
}

type leadRow struct {
	Asked, AskedAt, Name, Email, Company, Outcome string
}

// view gathers the operator's page: the demos, their limits and leads.
func (a *App) view(ctx context.Context, notice string, leads []cloud.Lead) page {
	now := a.Now()
	view := page{
		Page: "operator", Title: "gusto operator", Notice: notice,
		Paused: a.Store.DemosPaused(ctx), Limits: a.Store.DemoLimits(ctx), LimitsSet: a.Store.DemoLimitsSet(ctx),
	}
	for _, demo := range a.running(ctx) {
		row := runningRow{Name: demo.Name, Company: demo.Company, State: "starting", Left: "not started"}
		if demo.Ready {
			row.State, row.Left = "in use", left(demo.Left)
		}
		view.Running = append(view.Running, row)
	}
	for _, lead := range leads {
		view.Leads = append(view.Leads, leadRow{
			Asked: ago(lead.Requested, now), AskedAt: lead.Requested.Format("2 Jan 2006 15:04 MST"),
			Name: lead.Name, Email: lead.Email, Company: lead.Company, Outcome: outcome(lead),
		})
	}
	return view
}

// outcome is where a demo a lead asked for got to: starting, in use, or why it ended, and whether it was extended.
func outcome(lead cloud.Lead) string {
	words := lead.Reason
	if lead.Ended.IsZero() {
		words = "in use"
		if lead.Ready.IsZero() {
			words = "starting"
		}
	}
	if lead.Extended {
		words += " · extended"
	}
	return words
}

// draw answers with the operator's page.
func draw(w http.ResponseWriter, view page) {
	data, err := json.Marshal(view)
	if err != nil {
		http.Error(w, "the operator page was not drawn", http.StatusInternalServerError)
		return
	}
	head := `<title>gusto operator</title><meta name="robots" content="noindex,nofollow">` + web.SharedHead()
	web.WriteAppPage(w, http.StatusOK, web.CloudEntry, "", head, string(data))
}
