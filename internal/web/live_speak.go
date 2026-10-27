package web

import (
	"net/http"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/roomlive"
)

// speak is somebody saying something in a room: a turn for each agent it mentions, for the only agent when there is one and none
// is mentioned, and otherwise something the room reads and no agent answers. It gives the id of the first turn made, or of the
// note.
//
// With a key, saying it again under the same key makes nothing new. The note is found by its key. A turn is found by its key
// too, but that is a search of every job, so it is made only for a request its sender says is a resend, which a sender only
// does after losing the answer, and which is the case a restart in between makes.
func (s *Server) speak(asked string, held instance.Instance, thread instance.Thread, said, key string, resend bool) (string, error) {
	held = s.liveRoom(held)
	agents := mentioned(held, said)
	waiting := mentionedWaiting(held, said)
	if working := held.Working(); len(agents) == 0 && len(waiting) == 0 && len(working) == 1 {
		agents = working
	}
	if len(agents) == 0 {
		note, err := s.instances.NoteKeyed(held.ID, thread.ID, asked, said, key)
		if err != nil {
			return "", err
		}
		return note.ID, s.sayWaiting(held, thread, waiting)
	}
	if resend && key != "" {
		s.keyed.Lock()
		defer s.keyed.Unlock()
	}
	first := ""
	for _, agent := range agents {
		turnKey := ""
		if key != "" {
			turnKey = key + "#" + agent.ID
		}
		if resend && turnKey != "" {
			if already, found := s.index.FindKey(asked, turnKey); found {
				first = firstOf(first, already.ID)
				continue
			}
		}
		if over := s.overAllowance(held, agent); over != "" {
			note, err := s.instances.Notice(held.ID, thread.ID, over)
			if err != nil {
				return "", err
			}
			first = firstOf(first, note.ID)
			continue
		}
		record, err := s.startRoomTurn(asked, held, thread, said, agent, roomAsk{answers: first, key: turnKey})
		if err != nil {
			return "", err
		}
		first = firstOf(first, record.ID)
	}
	return first, s.sayWaiting(held, thread, waiting)
}

// sayWaiting tells the room that the agents a message mentioned that no admin has accepted were not asked.
func (s *Server) sayWaiting(held instance.Instance, thread instance.Thread, waiting []instance.Agent) error {
	for _, agent := range waiting {
		if _, err := s.instances.Notice(held.ID, thread.ID, agent.Name+" is waiting for an admin of the room to accept it, so it was not asked."); err != nil {
			return err
		}
	}
	return nil
}

// liveSay is the say command: the page's form, sent over the socket.
func (s *Server) liveSay(r *http.Request, held instance.Instance, command roomlive.Command) roomlive.Result {
	said := strings.TrimSpace(command.Text)
	if said == "" {
		return roomlive.Result{Error: "there is nothing to say"}
	}
	thread, found := s.instances.Thread(held.ID, command.Thread)
	if !found {
		return roomlive.Result{Error: "that conversation is not here"}
	}
	ref, err := s.speak(s.keeper.who(r), held, thread, said, command.ID, command.Retry)
	if err != nil {
		return roomlive.Result{Error: err.Error()}
	}
	return roomlive.Result{OK: true, Ref: ref}
}

// liveStop is the Stop button, sent over the socket. It stops a turn of this room and no other.
func (s *Server) liveStop(r *http.Request, held instance.Instance, command roomlive.Command) roomlive.Result {
	record, found := s.index.Get(command.Ref)
	if !found || record.Instance != held.ID {
		return roomlive.Result{Error: "that turn is not in this room"}
	}
	stopping := r.Clone(r.Context())
	stopping.SetPathValue("id", record.ID)
	stopped, _, status := s.stopTurn(stopping, s.keeper.who(r))
	switch status {
	case http.StatusOK, http.StatusConflict:
		return roomlive.Result{OK: true, Ref: stopped.ID, Dupe: status == http.StatusConflict}
	case http.StatusNotFound:
		return roomlive.Result{Error: "that turn is not in this room"}
	}
	return roomlive.Result{Error: "that turn could not be stopped"}
}

// liveSeen is a person looking at a conversation up to a time, which is what the unread counts are kept from. A time later than
// now is now.
func (s *Server) liveSeen(r *http.Request, held instance.Instance, command roomlive.Command) roomlive.Result {
	thread, found := s.instances.Thread(held.ID, command.Thread)
	if !found {
		return roomlive.Result{Error: "that conversation is not here"}
	}
	now := time.Now().Unix()
	at := command.At
	if at <= 0 || at > now {
		at = now
	}
	if err := s.instances.Saw(held.ID, thread.ID, s.askerName(r), at); err != nil {
		return roomlive.Result{Error: err.Error()}
	}
	return roomlive.Result{OK: true}
}
