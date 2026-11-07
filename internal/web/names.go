package web

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/roster"
)

// rosterBook is the organization's people as the roster gusto cloud writes says: what each is called and their role. A
// page asks for names many times over, and every request asks for its person's role, so the roster is looked at again at most
// once a second, and read again only when it changed.
type rosterBook struct {
	mu      sync.Mutex
	checked time.Time
	stamp   time.Time
	size    int64
	byEmail map[string]roster.Member
}

// rosterRecheck is how long a roster book trusts what it read before it looks at the roster again, which is also how long a
// person who was removed may still be served.
const rosterRecheck = time.Second

// member is a person as the newest roster that could be read says. A roster that changed and cannot be read keeps the
// people known from the one before, which holds every change gusto cloud wrote up to it.
func (b *rosterBook) member(path, who string) (roster.Member, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if now := time.Now(); now.Sub(b.checked) >= rosterRecheck {
		b.checked = now
		b.refresh(path)
	}
	member, found := b.byEmail[strings.ToLower(who)]
	return member, found
}

// of is the name the roster gives a person, when it gives one.
func (b *rosterBook) of(path, who string) (string, bool) {
	if path == "" {
		return "", false
	}
	member, found := b.member(path, who)
	name := strings.TrimSpace(member.Name)
	return name, found && name != ""
}

// roleOf is a person's role as the roster says it now, and false for a person not in it, and for anybody before a roster
// could be read at all.
func (b *rosterBook) roleOf(path, who string) (Role, bool) {
	member, found := b.member(path, who)
	if !found {
		return "", false
	}
	if Role(member.Role) == Admin {
		return Admin, true
	}
	return Runner, true
}

// refresh reads the roster when its file is not the one read last.
func (b *rosterBook) refresh(path string) {
	info, err := os.Stat(path)
	if err != nil || (info.ModTime().Equal(b.stamp) && info.Size() == b.size) {
		return
	}
	list, err := roster.Read(path)
	if err != nil {
		return
	}
	members := make(map[string]roster.Member, len(list.Members))
	for _, member := range list.Members {
		members[strings.ToLower(member.Email)] = member
	}
	b.byEmail, b.stamp, b.size = members, info.ModTime(), info.Size()
}

// personName is what the organization calls a person: the name they gave, else what their address says.
func (s *Server) personName(who string) string {
	if name, found := s.names.of(s.rosterPath, who); found {
		return name
	}
	return nameFromAddress(who)
}

// memberNow is the role a person has now, as gusto cloud's roster says, for a server behind gusto cloud. checked is false
// for a server that has no roster to ask, whose sign-ins stand for as long as they last.
func (s *Server) memberNow(who string) (role Role, in, checked bool) {
	if s.front == nil || s.rosterPath == "" || who == sharedSignIn {
		return "", false, false
	}
	role, in = s.names.roleOf(s.rosterPath, who)
	return role, in, true
}
