package web

import (
	"hash/fnv"
	"net/http"
	"strconv"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/instance"
)

// A room has three settings besides its people and agents: its guidelines, which its admins write and every agent in it is given at
// the start of each turn; its mute, which each person sets for themselves; and its archive, which an admin sets for everybody.

func (s *Server) roomSettingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /rooms/{id}/guidelines", s.guard(s.setRoomGuidelines))
	mux.HandleFunc("POST /rooms/{id}/mute", s.guard(s.muteRoom))
	mux.HandleFunc("POST /rooms/{id}/archive", s.guard(s.archiveRoom))
}

func init() { helpers["maxGuidelines"] = func() int { return instance.MaxGuidelines } }

// roomArchivedWords is why nothing is said in an archived room.
const roomArchivedWords = "this room is archived, so nothing can be said in it until an admin brings it back"

// roomHome is the room's newest conversation, where a change to its settings comes back to.
func (s *Server) roomHome(held instance.Instance) string {
	return threadPath(s.instances.Latest(held.ID))
}

func (s *Server) setRoomGuidelines(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	if !s.runsRoom(r, held) {
		http.Error(w, "only a room's admin changes its guidelines", http.StatusForbidden)
		return
	}
	text := r.FormValue("guidelines")
	if err := s.instances.SetGuidelines(held.ID, text); err != nil {
		s.turnAway(w, r, s.roomHome(held)+"?guidelines=1", err.Error())
		return
	}
	now, _ := s.instances.Get(held.ID)
	if now.Guidelines != held.Guidelines {
		s.note(r, changelog.RoomGuidelines, map[string]string{"room": held.Title, "was": held.Guidelines, "now": now.Guidelines})
	}
	http.Redirect(w, r, s.roomHome(held), http.StatusSeeOther)
}

// muteRoom mutes or unmutes the room for the person asking, who has to be in it.
func (s *Server) muteRoom(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	me := s.askerName(r)
	if held.RoleOf(me) == "" {
		http.Error(w, "only somebody in a room mutes it", http.StatusForbidden)
		return
	}
	on := r.FormValue("mute") == "on"
	if err := s.instances.SetMuted(held.ID, me, on); err != nil {
		s.turnAway(w, r, s.roomHome(held), err.Error())
		return
	}
	if held.MutedBy(me) != on {
		s.note(r, changelog.RoomMuted, map[string]string{"room": held.Title, "now": onOff(on)})
	}
	http.Redirect(w, r, s.roomHome(held), http.StatusSeeOther)
}

// archiveRoom archives the room for everybody in it, or brings it back.
func (s *Server) archiveRoom(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	if !s.runsRoom(r, held) {
		http.Error(w, "only a room's admin archives it", http.StatusForbidden)
		return
	}
	on := r.FormValue("archive") == "on"
	if err := s.instances.SetArchived(held.ID, on); err != nil {
		s.turnAway(w, r, s.roomHome(held), err.Error())
		return
	}
	if (held.Archived != 0) != on {
		s.note(r, changelog.RoomArchived, map[string]string{"room": held.Title, "now": onOff(on)})
	}
	http.Redirect(w, r, s.roomHome(held), http.StatusSeeOther)
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// roomSettings is what a conversation's page shows of its room's settings, for the person looking.
type roomSettings struct {
	// Archived says nothing can be said in the room. Runs says the person may change its guidelines and archive it, In that they are
	// in it, and Muted that they muted it.
	Archived, Runs, In, Muted bool
	// Mark changes when the guidelines or the archive do, and Refused is why the last change to the guidelines was refused.
	Mark, Refused string
}

func (s *Server) roomSettingsOf(r *http.Request, held instance.Instance) roomSettings {
	me := s.askerName(r)
	return roomSettings{Archived: held.Archived != 0, Runs: s.runsRoom(r, held), In: held.RoleOf(me) != "", Muted: held.MutedBy(me),
		Mark: roomSettingsMark(held), Refused: s.refusedIn(r, "guidelines")}
}

// roomSettingsMark stands for the parts of a room that a page draws once, its guidelines and whether it is archived, so a page that
// is told the room changed can tell whether it has to be drawn again.
func roomSettingsMark(held instance.Instance) string {
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(held.Guidelines))
	return strconv.FormatBool(held.Archived != 0) + "-" + strconv.FormatUint(uint64(sum.Sum32()), 36)
}
