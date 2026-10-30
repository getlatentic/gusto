package web

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/roster"
)

// nameBook is what each person of the organization is called, as the roster the front door writes says. A page asks for
// names many times over, so the roster is looked at again at most once a second, and read again only when it changed.
type nameBook struct {
	mu      sync.Mutex
	checked time.Time
	stamp   time.Time
	size    int64
	byEmail map[string]string
}

// nameRecheck is how long a name book trusts what it read before it looks at the roster again.
const nameRecheck = time.Second

// of is the name the roster gives a person, when it gives one.
func (b *nameBook) of(path, who string) (string, bool) {
	if path == "" {
		return "", false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if now := time.Now(); now.Sub(b.checked) >= nameRecheck {
		b.checked = now
		b.refresh(path)
	}
	name, found := b.byEmail[strings.ToLower(who)]
	return name, found && name != ""
}

// refresh reads the roster when its file is not the one read last. A roster that cannot be read keeps the names known.
func (b *nameBook) refresh(path string) {
	info, err := os.Stat(path)
	if err != nil || (info.ModTime().Equal(b.stamp) && info.Size() == b.size) {
		return
	}
	list, err := roster.Read(path)
	if err != nil {
		return
	}
	names := make(map[string]string, len(list.Members))
	for _, member := range list.Members {
		names[strings.ToLower(member.Email)] = strings.TrimSpace(member.Name)
	}
	b.byEmail, b.stamp, b.size = names, info.ModTime(), info.Size()
}

// personName is what the organization calls a person: the name they gave, else what their address says.
func (s *Server) personName(who string) string {
	if name, found := s.names.of(s.rosterPath, who); found {
		return name
	}
	return nameFromAddress(who)
}
