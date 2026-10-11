package web

import (
	"net/http"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/industry"
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
	harness, model := defaultPick(picks, s.lastUsed(r), s.orgDefault())
	s.render(w, "chat.html", map[string]any{
		"Title":      productName + " — chat",
		"Nav":        "chat",
		"Chat":       true,
		"Picks":      picks,
		"Harness":    harness,
		"Model":      model,
		"Agents":     s.agentOptions(picks),
		"Models":     s.modelOptions(picks),
		"Rail":       s.railFor(r, "/chat"),
		"Here":       "/chat",
		"Refused":    r.URL.Query().Get("refused"),
		"Start":      s.startIn(r),
		"Roles":      s.roleChoices(r),
		"Role":       s.chosenRole(r),
		"NeedsModel": s.needsModel(r),
		"Presets":    modelPresets,
		"Missing":    s.missingEngines(),
		"FoundKeys":  s.foundKeys,
	})
}

// startFolder is a folder a chat page opens with already chosen: the project the person pressed New chat under in the side list.
type startFolder struct{ At, Name, Harness string }

func (s *Server) startIn(r *http.Request) startFolder {
	id, dir := splitAt(r.URL.Query().Get("at"))
	held, found := s.folderAgent(r, id)
	if !found {
		return startFolder{}
	}
	if _, ok := s.workspaceDir(id, dir); !ok {
		return startFolder{}
	}
	name := s.agentOf(held).Name
	if dir != "" {
		name = path.Base(dir)
	}
	return startFolder{At: path.Join(id, dir), Name: name, Harness: s.engineOf(id, held.Harness)}
}

// picksFor is each harness this deployment can run for this person, with the models it may use.
func (s *Server) picksFor(r *http.Request) []pick {
	if s.limitedAway(r) {
		return nil
	}
	offered := s.offeredTo(r)
	var picks []pick
	for _, entry := range s.readyEngines() {
		picks = append(picks, pick{Entry: entry, Models: offered})
	}
	return picks
}

// defaultPick is the harness and model last used that can still be used, else the organisation's default when it can run, else the
// first harness and its first model.
func defaultPick(picks []pick, recent []yours, org chatDefault) (harness, model string) {
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
	for _, candidate := range picks {
		if candidate.Name == org.Harness {
			if slices.Contains(candidate.Models, org.Model) {
				return org.Harness, org.Model
			}
			return org.Harness, first(candidate.Models)
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
	held, thread, why := s.threadForChat(r)
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

// threadForChat is where a chat goes: a folder of the person's cloud computer, a folder of their own computer kept the same as an
// agent's, or the agent and model chosen.
func (s *Server) threadForChat(r *http.Request) (instance.Instance, instance.Thread, string) {
	model := strings.TrimSpace(r.FormValue("model"))
	if at := strings.TrimSpace(r.FormValue("at")); at != "" {
		return s.cloudFolderThread(r, at, model)
	}
	return s.chatThread(r, strings.TrimSpace(r.FormValue("harness")), model, folderOf(r), s.roleOf(r))
}

// chatThread is the agent and the empty conversation a new chat goes into, on the model chosen, or why there is none.
func (s *Server) chatThread(r *http.Request, harness, model string, where folder, role *industry.Template) (instance.Instance, instance.Thread, string) {
	setup := ""
	wanted := instance.Instance{Harness: harness, Model: model, Who: s.keeper.who(r), Title: where.Name, Folder: where.Key}
	if role != nil {
		if role.Restricted() && !s.inBuilding.Load() {
			return instance.Instance{}, instance.Thread{}, "this work has to stay in your network, and no model here is"
		}
		if err := s.install(*role); err != nil {
			return instance.Instance{}, instance.Thread{}, err.Error()
		}
		setup = role.Name
		wanted.Setup, wanted.Sensitivity, wanted.Title = role.Name, string(role.Sensitivity), role.Title
	}
	held, found := s.agentFor(r, harness, setup, where)
	if !found {
		started, _, why := s.beginInstance(r, wanted)
		if why != "" {
			return started, instance.Thread{}, why
		}
		return started, s.instances.Latest(started.ID), ""
	}
	if why := s.unusable(r, model); why != "" {
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

// folder is a folder on the person's computer that a chat works in: the key the desktop app made of its path, and its name.
type folder struct{ Key, Name string }

var folderKey = regexp.MustCompile(`^[a-f0-9]{16}$`)

// folderOf is the folder a chat was started for, or none. A key that is not one the app makes is no folder.
func folderOf(r *http.Request) folder {
	key := strings.TrimSpace(r.FormValue("folder"))
	if !folderKey.MatchString(key) {
		return folder{}
	}
	name := strings.TrimSpace(strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return -1
		}
		return c
	}, r.FormValue("folder_name")))
	if name == "" {
		name = "Folder"
	}
	if runes := []rune(name); len(runes) > 80 {
		name = string(runes[:80])
	}
	return folder{Key: key, Name: name}
}

// agentFor is the agent this person used last for a harness that was started for the same folder, or for none, which is what a chat
// with no folder is: no repository, setup or sensitivity either.
func (s *Server) agentFor(r *http.Request, harness, setup string, where folder) (instance.Instance, bool) {
	var best instance.Instance
	var seen int64
	for _, each := range s.ownInstances(r, s.instances.List()) {
		fits := each.Setup == setup && (setup != "" || each.Sensitivity == "")
		if each.Harness != harness || each.Folder != where.Key || each.Repo != "" || !fits {
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
