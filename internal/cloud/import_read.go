package cloud

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// legacy is what the files an earlier release kept under the root hold, read and checked but not
// yet in the database. Nothing here is written back: the files are never changed or removed.
type legacy struct {
	orgs        []Org
	people      []Person
	invitations []legacyInvitation
	links       []legacyLink
	leads       []Lead
	runtimes    map[string]Runtime
	limits      *DemoLimits
	paused      bool
	key         []byte
	skipped     map[string]int
}

type legacyInvitation struct {
	org string
	Invitation
}

type legacyLink struct {
	hash string
	link
}

// reader reads the old layout, counting what it leaves out and why.
type reader struct {
	root    string
	now     time.Time
	skipped map[string]int
}

func (r *reader) skip(kind, reason, path string) {
	r.skipped[kind+" "+reason]++
	level := slog.LevelWarn
	if reason == "expired" {
		level = slog.LevelDebug
	}
	slog.Log(context.Background(), level, "a file of the old accounts layout was left out of the import", "kind", kind, "reason", reason, "file", path)
}

// readLegacy reads every record of the old layout under root. A file that cannot be read, or whose
// record cannot stand in the database, is counted and left out; the rest are read.
func readLegacy(root string, now time.Time) legacy {
	r := &reader{root: root, now: now, skipped: map[string]int{}}
	l := legacy{runtimes: map[string]Runtime{}, skipped: r.skipped}
	l.orgs = r.orgs()
	slugs := map[string]bool{}
	for _, org := range l.orgs {
		slugs[org.Slug] = true
	}
	l.people = r.people(slugs)
	l.invitations = r.invitations(slugs)
	l.links = r.links()
	l.leads = r.leads()
	l.runtimes = r.runtimes(slugs)
	l.limits, l.paused, l.key = r.settings()
	return l
}

// jsonFiles reads each .json file in a directory into a T, in name order. A directory that is not there has none.
func jsonFiles[T any](r *reader, kind, dir string) (names []string, values []T) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".json") {
			continue
		}
		var value T
		if err := readFile(filepath.Join(dir, name), &value); err != nil {
			r.skip(kind, "unreadable", filepath.Join(dir, name))
			continue
		}
		names, values = append(names, strings.TrimSuffix(name, ".json")), append(values, value)
	}
	return names, values
}

func (r *reader) orgs() []Org {
	entries, err := os.ReadDir(filepath.Join(r.root, "orgs"))
	if err != nil {
		return nil
	}
	var out []Org
	for _, entry := range entries {
		var org Org
		path := filepath.Join(r.root, "orgs", entry.Name(), "org.json")
		if _, statErr := os.Stat(path); statErr != nil {
			continue
		}
		switch err := readFile(path, &org); {
		case err != nil:
			r.skip("organisations", "unreadable", path)
		case org.Slug != entry.Name() || org.Created.IsZero() || org.Admin == "":
			r.skip("organisations", "mismatched", path)
		default:
			out = append(out, org)
		}
	}
	return out
}

func (r *reader) people(slugs map[string]bool) []Person {
	_, all := jsonFiles[Person](r, "people", filepath.Join(r.root, "people"))
	var out []Person
	for _, person := range all {
		person.Email = strings.ToLower(strings.TrimSpace(person.Email))
		switch {
		case person.Email == "" || person.Created.IsZero():
			r.skip("people", "unreadable", person.Email)
		case !slugs[person.Org]:
			r.skip("people", "orphaned", person.Email)
		default:
			out = append(out, person)
		}
	}
	return out
}

func (r *reader) invitations(slugs map[string]bool) []legacyInvitation {
	var out []legacyInvitation
	for slug := range slugs {
		_, all := jsonFiles[Invitation](r, "invitations", filepath.Join(r.root, "orgs", slug, "invites"))
		for _, invitation := range all {
			switch {
			case invitation.Email == "" || invitation.Link == "" || invitation.Created.IsZero() || invitation.Expires.IsZero():
				r.skip("invitations", "unreadable", slug+"/"+invitation.Email)
			case !r.now.Before(invitation.Expires):
				r.skip("invitations", "expired", slug+"/"+invitation.Email)
			default:
				out = append(out, legacyInvitation{org: slug, Invitation: invitation})
			}
		}
	}
	return out
}

func (r *reader) links() []legacyLink {
	names, all := jsonFiles[link](r, "links", filepath.Join(r.root, "tokens"))
	var out []legacyLink
	for i, made := range all {
		switch {
		case len(names[i]) != 64 || made.Email == "" || (made.Purpose != Verify && made.Purpose != SignIn && made.Purpose != Invite):
			r.skip("links", "unreadable", names[i])
		case !r.now.Before(made.Expires):
			r.skip("links", "expired", names[i])
		default:
			out = append(out, legacyLink{hash: names[i], link: made})
		}
	}
	return out
}

func (r *reader) leads() []Lead {
	_, all := jsonFiles[Lead](r, "leads", filepath.Join(r.root, "demos"))
	var out []Lead
	for _, lead := range all {
		if lead.Slug == "" || lead.Requested.IsZero() {
			r.skip("leads", "unreadable", lead.Slug)
			continue
		}
		out = append(out, lead)
	}
	return out
}

func (r *reader) runtimes(slugs map[string]bool) map[string]Runtime {
	out := map[string]Runtime{}
	for slug := range slugs {
		path := filepath.Join(r.root, "orgs", slug, "runtime.json")
		var rt Runtime
		switch err := readFile(path, &rt); {
		case missing(err):
		case err != nil || rt.Updated.IsZero():
			r.skip("runtime", "unreadable", path)
		default:
			out[slug] = rt
		}
	}
	return out
}

// settings reads the demo limits the owner set, the file whose presence pauses demos, and the key
// cookies are signed with.
func (r *reader) settings() (limits *DemoLimits, paused bool, key []byte) {
	var kept DemoLimits
	switch err := readFile(filepath.Join(r.root, "demo-limits.json"), &kept); {
	case err == nil:
		limits = &kept
	case !missing(err):
		r.skip("settings", "unreadable", "demo-limits.json")
	}
	_, statErr := os.Stat(filepath.Join(r.root, "demos.off"))
	paused = statErr == nil
	if text, err := os.ReadFile(filepath.Join(r.root, "secret")); err == nil && len(text) >= 32 {
		key = text
	}
	return limits, paused, key
}
