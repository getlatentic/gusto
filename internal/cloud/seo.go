package cloud

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
)

const (
	siteName        = "gusto"
	landingTitle    = siteName + " — one place for every AI agent your team needs"
	demoFormTitle   = "Start a demo of " + siteName
	organisationURL = "https://getlatentic.com"
)

// seen is what a search engine is told about a page it may list: its address, what it is about, and the image a link to it shows.
func (s *Server) seen(page view, path, description string) view {
	page.Index, page.Description = true, description
	page.Canonical, page.Share = s.URL.JoinPath(path).String(), s.URL.JoinPath("/static/og.png").String()
	return page
}

// landingDescription is what the landing page says it is, for the page's description and for a link to it.
func landingDescription(page view) string {
	return "Run Claude Code, Codex, Pi, OpenCode and other agents with the same controls for credentials, models, spend and oversight. Try it for " + minutes(page.DemoLeft) + "."
}

// structured is what the landing page says about who makes it and what it is, as data a search engine reads. It says only what the
// page says: there is no price, rating or count in it.
func (s *Server) structured(description string) template.JS {
	publisher := map[string]any{"@type": "Organization", "name": "Latentic", "url": organisationURL}
	text, _ := json.Marshal(map[string]any{"@context": "https://schema.org", "@graph": []any{
		map[string]any{"@type": "Organization", "@id": organisationURL + "#organization", "name": "Latentic", "url": organisationURL,
			"logo": s.URL.JoinPath("/static/mark.png").String()},
		map[string]any{"@type": "SoftwareApplication", "name": siteName, "url": s.URL.JoinPath("/").String(), "applicationCategory": "BusinessApplication",
			"operatingSystem": "Web", "description": description, "publisher": publisher},
	}})
	// Marshal writes < > & as escapes, so the data cannot end its own script.
	return template.JS(text)
}

// hiddenPaths are the pages of a signed-in person, which a search engine has no use for.
var hiddenPaths = []string{"/signin", "/signup", "/verify", "/org", "/open", "/invite", "/lease", "/demo/wait", "/demo/end", "/demo/ended/", "/static/"}

// robots tells a search engine what it may read here: the landing page and the demo form, not the pages behind them.
func (s *Server) robots(w http.ResponseWriter, _ *http.Request) {
	var out strings.Builder
	out.WriteString("User-agent: *\nAllow: /\n")
	for _, path := range hiddenPaths {
		if path != "/static/" {
			fmt.Fprintf(&out, "Disallow: %s\n", path)
		}
	}
	out.WriteString("Allow: /static/og.png\nSitemap: " + s.URL.JoinPath("/sitemap.xml").String() + "\n")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write([]byte(out.String()))
}

// sitemap lists the pages worth finding: the landing page, and the demo form where a demo is offered.
func (s *Server) sitemap(w http.ResponseWriter, _ *http.Request) {
	paths := []string{"/"}
	if s.demoOffered() {
		paths = append(paths, "/demo")
	}
	var out strings.Builder
	out.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, path := range paths {
		out.WriteString("  <url><loc>" + template.HTMLEscapeString(s.URL.JoinPath(path).String()) + "</loc></url>\n")
	}
	out.WriteString("</urlset>\n")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write([]byte(out.String()))
}

// unlisted is what every organisation's address, and every app an agent shares, tells a search engine: nothing here is for listing.
func unlisted(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	if r.Method == http.MethodGet && r.URL.Path == "/robots.txt" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("User-agent: *\nDisallow: /\n"))
		return true
	}
	return false
}
