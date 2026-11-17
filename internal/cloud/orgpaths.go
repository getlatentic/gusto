package cloud

import (
	"cmp"
	"net/http"
	"net/url"
	"strings"
)

// orgPath is where an organization's server is reached on gusto cloud's own name when organizations are at paths: its pages
// are under /workspace/<slug>.
const orgPath = "/workspace/"

// orgAtPath is the organization a path is under, and the path its own server is asked for, when organizations are at paths.
func (s *Server) orgAtPath(path string) (slug, rest string, ok bool) {
	if !s.OrgPaths || s.Controller == nil {
		return "", "", false
	}
	tail, found := strings.CutPrefix(path, orgPath)
	if !found {
		return "", "", false
	}
	slug, rest, _ = strings.Cut(tail, "/")
	if ValidSlug(slug) != nil {
		return "", "", false
	}
	return slug, "/" + rest, true
}

// mountedRequest is a request for a page under an organization's path, as its server is asked for it: the same request at
// the path below the organization's.
func mountedRequest(r *http.Request, slug, rest string) *http.Request {
	inner := *r.URL
	inner.Path = rest
	inner.RawPath = ""
	if escaped, found := strings.CutPrefix(r.URL.EscapedPath(), orgPath+slug); found && escaped != "" {
		inner.RawPath = escaped
	}
	mounted := r.Clone(r.Context())
	mounted.URL = &inner
	mounted.RequestURI = inner.RequestURI()
	return mounted
}

// orgAt is the organization an address of a page is one of, by how organizations are reached here: its host name, or its path
// on gusto cloud's own name.
func (s *Server) orgAt(page *url.URL) (string, bool) {
	if page.Scheme != s.URL.Scheme || page.Port() != s.URL.Port() {
		return "", false
	}
	if s.OrgPaths {
		if !strings.EqualFold(page.Hostname(), s.URL.Hostname()) {
			return "", false
		}
		slug, _, ok := s.orgAtPath(page.Path)
		return slug, ok
	}
	return s.orgSlug(page.Host)
}

// orgShown is an organization's address as a person reads it, with no scheme.
func (s *Server) orgShown(slug string) string {
	address := s.orgURL(slug)
	_, rest, _ := strings.Cut(address, "://")
	return cmp.Or(rest, address)
}

// mountedAnswer fits an organization's answer to the path it is reached at: a redirect to one of its own pages goes under
// the path. The server keeps its cookies to the path itself (web.Server.UseAddress).
func mountedAnswer(prefix string) func(*http.Response) error {
	return func(answer *http.Response) error {
		if to := answer.Header.Get("Location"); strings.HasPrefix(to, "/") && !strings.HasPrefix(to, "//") {
			answer.Header.Set("Location", prefix+to)
		}
		return nil
	}
}
