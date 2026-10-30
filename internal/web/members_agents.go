package web

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/instance"
)

// An agent starts private to the person who made it. They choose who else may work with it, and they or an admin of this server
// change it or remove it.

func (s *Server) makeAgent(w http.ResponseWriter, r *http.Request) {
	if s.crew == nil {
		s.turnAway(w, r, "/members", "this server keeps no agents")
		return
	}
	wanted, err := s.agentFromForm(r, crew.Agent{})
	if err != nil {
		s.turnAway(w, r, "/members?new=1", err.Error())
		return
	}
	wanted.Owner = s.askerName(r)
	made, err := s.crew.Make(wanted)
	if err != nil {
		s.turnAway(w, r, "/members?new=1", err.Error())
		return
	}
	http.Redirect(w, r, "/members/agents/"+made.ID, http.StatusSeeOther)
}

// agentFromForm is an agent as the form changes it. The agent type is chosen once, when it is made, since its conversations are
// that agent type's own; the model, the skills and the rest can change.
func (s *Server) agentFromForm(r *http.Request, held crew.Agent) (crew.Agent, error) {
	if r.Form == nil {
		_ = r.ParseForm()
	}
	if r.Form.Has("name") {
		held.Name = r.FormValue("name")
	}
	if r.Form.Has("specialty") {
		held.Specialty = strings.TrimSpace(r.FormValue("specialty"))
	}
	if r.Form.Has("persona") {
		held.Persona = strings.TrimSpace(r.FormValue("persona"))
	}
	if held.ID == "" {
		held.Harness = r.FormValue("harness")
		if !slices.ContainsFunc(s.picksFor(r), func(each pick) bool { return each.Name == held.Harness }) {
			return held, errors.New("choose an agent type this server can run")
		}
	}
	if r.Form.Has("model") {
		held.Model = r.FormValue("model")
		if held.Model != "" && !s.offeredTo(r).Offers(held.Model) {
			return held, errors.New("that model is not one you may use")
		}
	}
	if held.ID == "" && held.Model == "" {
		held.Model = s.modelToStartOn(r, held.Harness)
	}
	if r.Form.Has("setup") {
		if err := s.giveSkills(&held, r.FormValue("setup")); err != nil {
			return held, err
		}
	}
	return held, nil
}

// giveSkills gives an agent a ready-made role or a skill set. A role is put in place as a skill set the first time it is given.
func (s *Server) giveSkills(held *crew.Agent, name string) error {
	if name == "" {
		held.Setup = ""
		return nil
	}
	if role, found := industry.Get(name); found {
		if role.Restricted() && !s.inBuilding.Load() {
			return errors.New("that role's work has to stay in your network, and no model here is")
		}
		if err := s.install(role); err != nil {
			return err
		}
		held.Setup = name
		return nil
	}
	if _, found := s.skillsOf(name); !found {
		return errors.New("there is no skill set called " + name)
	}
	held.Setup = name
	return nil
}

// ownAgent is the agent the path names when the person asking made it or runs this server.
func (s *Server) ownAgent(w http.ResponseWriter, r *http.Request) (crew.Agent, bool) {
	member, found := s.crewAgent(r)
	if !found {
		http.NotFound(w, r)
		return crew.Agent{}, false
	}
	if !strings.EqualFold(member.Owner, s.askerName(r)) && !s.admin(r) {
		http.Error(w, "only the person who made an agent changes it", http.StatusForbidden)
		return crew.Agent{}, false
	}
	return member, true
}

func (s *Server) changeAgent(w http.ResponseWriter, r *http.Request) {
	member, found := s.ownAgent(w, r)
	if !found {
		return
	}
	back := "/members/agents/" + member.ID + "?tab=" + r.FormValue("tab")
	changed, err := s.agentFromForm(r, member)
	if err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	if _, err := s.crew.Change(member.ID, func(held *crew.Agent) error { *held = changed; return nil }); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// changeAgentAccess lets another person work with an agent, or stops them.
func (s *Server) changeAgentAccess(w http.ResponseWriter, r *http.Request) {
	member, found := s.ownAgent(w, r)
	if !found {
		return
	}
	back := "/members/agents/" + member.ID + "?tab=access"
	who := strings.ToLower(strings.TrimSpace(r.FormValue("who")))
	if !strings.Contains(who, "@") && who != sharedSignIn {
		s.turnAway(w, r, back, "give the person's email address")
		return
	}
	_, err := s.crew.Change(member.ID, func(held *crew.Agent) error {
		held.Access = slices.DeleteFunc(held.Access, func(each string) bool { return each == who })
		if r.FormValue("action") != "remove" {
			held.Access = append(held.Access, who)
		}
		return nil
	})
	if err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// removeAgent deletes an agent. The rooms it was in keep what it said, and it answers there no more.
func (s *Server) removeAgent(w http.ResponseWriter, r *http.Request) {
	member, found := s.ownAgent(w, r)
	if !found {
		return
	}
	for _, held := range s.instances.List() {
		for _, agent := range held.Agents {
			if agent.Member == member.ID {
				_ = s.instances.RemoveAgent(held.ID, agent.ID)
			}
		}
	}
	if err := s.crew.Remove(member.ID); err != nil {
		s.turnAway(w, r, "/members/agents/"+member.ID, err.Error())
		return
	}
	http.Redirect(w, r, "/members", http.StatusSeeOther)
}

// messageAgent opens the person's own room with an agent, making it the first time: a room of two, where the agent answers
// everything without being mentioned, since it is the only one there.
func (s *Server) messageAgent(w http.ResponseWriter, r *http.Request) {
	member, found := s.crewAgent(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	who := s.askerName(r)
	if !member.MayUse(who) {
		http.Error(w, "you may not work with this agent; ask the person who made it", http.StatusForbidden)
		return
	}
	if held, found := s.directRoom(who, member.ID); found {
		http.Redirect(w, r, threadPath(s.instances.Latest(held.ID)), http.StatusSeeOther)
		return
	}
	held, err := s.instances.StartRoom(member.Name, who)
	if err == nil {
		_, err = s.instances.AddAgent(held.ID, instance.Agent{Name: member.Name, Harness: member.Harness, Model: member.Model, Member: member.ID})
	}
	if err == nil {
		err = s.instances.Direct(held.ID, true)
	}
	if err == nil {
		err = s.instances.Name(held.ID, s.instances.Latest(held.ID).ID, member.Name)
	}
	if err != nil {
		s.turnAway(w, r, "/members/agents/"+member.ID, err.Error())
		return
	}
	http.Redirect(w, r, threadPath(s.instances.Latest(held.ID)), http.StatusSeeOther)
}

// directRoom is the room of two a person keeps with an agent, when there is one.
func (s *Server) directRoom(who, member string) (instance.Instance, bool) {
	for _, held := range s.instances.RoomsOf(who) {
		if held.Direct && len(held.Members) == 1 && len(held.Agents) == 1 && held.Agents[0].Member == member {
			return held, true
		}
	}
	return instance.Instance{}, false
}

// modelToStartOn is the model a new agent runs on when nobody chose one. Where the server offers models, an agent type left to its
// own default would ask for a model nobody here provides and fail, so the organization's default is used when this agent type can run
// it, else the first model offered. Where the server offers none, the agent type's own default is right.
func (s *Server) modelToStartOn(r *http.Request, harness string) string {
	for _, each := range s.picksFor(r) {
		if each.Name != harness {
			continue
		}
		if org := s.orgDefault(); org.Harness == harness && slices.Contains(each.Models, org.Model) {
			return org.Model
		}
		return startingModel(each)
	}
	return ""
}
