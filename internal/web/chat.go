package web

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/instance"
)

// pick is one harness in the chat's chooser with the models it may be pointed at.
type pick struct {
	catalog.Entry
	Models []string
}

// chatPage is the place to start talking: a box, the harness and model it will
// go to, and the conversations already held. What it offers first is what the
// person used last, because somebody coming back wants to carry on.
func (s *Server) chatPage(w http.ResponseWriter, r *http.Request) {
	picks := s.picksFor(r)
	harness, model := defaultPick(picks, s.lastUsed(r))
	s.render(w, "chat.html", map[string]any{
		"Title":   productName + " — chat",
		"Nav":     "chat",
		"Chat":    true,
		"Picks":   picks,
		"Harness": harness,
		"Model":   model,
		"Agents":  s.agentOptions(picks),
		"Models":  s.modelOptions(picks),
		"Rail":    s.railFor(r),
		"Here":    "/chat",
		"Refused": r.URL.Query().Get("refused"),
	})
}

// picksFor is each harness this deployment can run for this person, with the models it may use.
func (s *Server) picksFor(r *http.Request) []pick {
	offered := s.offeredTo(r)
	var picks []pick
	for _, entry := range s.readyEngines() {
		if offered.Serves(entry.Name) && !s.limitedAway(r, entry.Name) {
			picks = append(picks, pick{Entry: entry, Models: offered.For(entry.Name)})
		}
	}
	return picks
}

// defaultPick is the harness and model last used that can still be used, or the first harness and its first model.
func defaultPick(picks []pick, recent []yours) (harness, model string) {
	for _, each := range recent {
		for _, candidate := range picks {
			if candidate.Name != each.Harness {
				continue
			}
			if each.Model == "" || slices.Contains(candidate.Models, each.Model) {
				return each.Harness, each.Model
			}
			return each.Harness, first(candidate.Models)
		}
	}
	if len(picks) == 0 {
		return "", ""
	}
	return picks[0].Name, first(picks[0].Models)
}

func first(all []string) string {
	if len(all) == 0 {
		return ""
	}
	return all[0]
}

// startChat sends what somebody wrote to the harness and model they chose. It carries on in the plain agent they
// already have for that harness, so what it worked on is still there, and starts one when they have none.
func (s *Server) startChat(w http.ResponseWriter, r *http.Request) {
	brief := strings.TrimSpace(r.FormValue("said"))
	if brief == "" {
		http.Redirect(w, r, "/chat", http.StatusSeeOther)
		return
	}
	held, thread, why := s.chatThread(r, strings.TrimSpace(r.FormValue("harness")), strings.TrimSpace(r.FormValue("model")))
	if why != "" {
		s.turnAway(w, r, "/chat", why)
		return
	}
	record, _, err := s.startTurn(s.keeper.who(r), held, thread, brief, "")
	if err != nil {
		s.turnAway(w, r, "/chat", err.Error())
		return
	}
	http.Redirect(w, r, threadPath(thread)+"#"+record.ID, http.StatusSeeOther)
}

// chatThread is the agent and the empty conversation a new chat goes into, on the model chosen, or why there is none.
func (s *Server) chatThread(r *http.Request, harness, model string) (instance.Instance, instance.Thread, string) {
	held, found := s.plainAgent(r, harness)
	if !found {
		started, _, why := s.beginInstance(r, instance.Instance{Harness: harness, Model: model, Who: s.keeper.who(r)})
		if why != "" {
			return started, instance.Thread{}, why
		}
		return started, s.instances.Latest(started.ID), ""
	}
	if why := s.unusable(r, held.Harness, model); why != "" {
		return held, instance.Thread{}, why
	}
	if model != "" && model != held.Model {
		if err := s.instances.SetModel(held.ID, model); err != nil {
			return held, instance.Thread{}, err.Error()
		}
		held.Model = model
	}
	thread := s.instances.Latest(held.ID)
	if len(thread.Turns) > 0 {
		var err error
		if thread, err = s.instances.NewThread(held.ID, ""); err != nil {
			return held, thread, err.Error()
		}
	}
	return held, thread, ""
}

// plainAgent is the agent this person used last for a harness that was started for no repository, setup or
// sensitivity, which is what a chat is.
func (s *Server) plainAgent(r *http.Request, harness string) (instance.Instance, bool) {
	var best instance.Instance
	var seen int64
	for _, each := range s.ownInstances(r, s.instances.List()) {
		if each.Harness != harness || each.Repo != "" || each.Setup != "" || each.Sensitivity != "" {
			continue
		}
		if active := s.lately(each).Active; best.ID == "" || active > seen {
			best, seen = each, active
		}
	}
	return best, best.ID != ""
}

// lastUsed is the person's agents, the one used last first.
func (s *Server) lastUsed(r *http.Request) []yours {
	held := s.ownInstances(r, s.instances.List())
	used := make([]yours, 0, len(held))
	for _, each := range held {
		used = append(used, s.lately(each))
	}
	sort.SliceStable(used, func(a, b int) bool { return used[a].Active > used[b].Active })
	return used
}

// option is one choice in a picker: what it is called, and a quieter word on what it costs or does.
type option struct {
	Value  string `json:"value"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

func (s *Server) modelOption(model string) option {
	return option{Value: model, Label: modelName(model), Detail: s.costOf(model)}
}

// modelOptions are the models of a harness as a picker lists them.
func (s *Server) modelOptionsOf(models []string) []option {
	options := make([]option, 0, len(models))
	for _, model := range models {
		options = append(options, s.modelOption(model))
	}
	return options
}

// agentOptions is the harnesses as JSON for the page's picker.
func (s *Server) agentOptions(picks []pick) string {
	options := make([]option, 0, len(picks))
	for _, each := range picks {
		label := each.Title
		if label == "" {
			label = each.Name
		}
		options = append(options, option{Value: each.Name, Label: label})
	}
	return asJSON(options)
}

// modelOptions is each harness's models as JSON, so choosing a harness changes what the other picker lists.
func (s *Server) modelOptions(picks []pick) string {
	byHarness := map[string][]option{}
	for _, each := range picks {
		byHarness[each.Name] = s.modelOptionsOf(each.Models)
	}
	return asJSON(byHarness)
}

func asJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(encoded)
}
