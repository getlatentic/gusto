package web

import "net/http"

// inviteOffer is the form that invites a person to the organization, which gusto cloud keeps: who is in it, and the invitation's mail
// and its link. This server keeps no list of its own, so the form is posted to gusto cloud, which asks again whether the person
// sending it may invite, and sends them to its page for the organization with the invitation said to be sent.
type inviteOffer struct {
	// Action is where gusto cloud takes the form, and Workspace the one the person invited joins.
	Action    string
	Workspace string
}

// inviteOffered is the invitation form for an admin of an organization that has gusto cloud, and nothing for anybody else: a server
// on its own has no organization to invite to, and people join a room there by address.
func (s *Server) inviteOffered(r *http.Request) *inviteOffer {
	if s.front == nil || s.front.URL == "" || !s.admin(r) {
		return nil
	}
	return &inviteOffer{Action: s.front.page("/org/invite", nil), Workspace: s.workspaceOf(r)}
}
