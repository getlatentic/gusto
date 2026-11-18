package cloud

import (
	"html"
	"strings"

	"github.com/getlatentic/gusto/internal/web"
)

const refresh = `<meta http-equiv="refresh" content="3">`

// pageHead is what the server writes into a page's head, so it is there before any of the page's code runs: the title, what a
// search engine and a link preview read, the head every page of the app shares, the refresh of a page that waits,
// and the lease banner where the host has one.
func pageHead(v view) string {
	var head strings.Builder
	head.WriteString(`<title>` + html.EscapeString(v.Title) + `</title>`)
	head.WriteString(seoTags(v))
	head.WriteString(web.SharedHead())
	switch {
	case v.Refresh:
		head.WriteString(refresh)
	case v.Watch != nil:
		// The page's code waits on the server and draws the page again when it changes; a browser running no code looks again.
		head.WriteString(`<noscript>` + refresh + `</noscript>`)
	}
	if v.Lease {
		head.WriteString(`<script src="` + web.Asset("/static/lease.js") + `" defer data-extend="/lease"></script>`)
	}
	return head.String()
}

// seoTags say whether the page may be listed, what it is, and its address and image.
func seoTags(v view) string {
	var tags strings.Builder
	meta := func(attr, key, content string) {
		tags.WriteString(`<meta ` + attr + `="` + key + `" content="` + html.EscapeString(content) + `">`)
	}
	if v.Index {
		meta("name", "robots", "index,follow,max-image-preview:large")
	} else {
		meta("name", "robots", "noindex,nofollow")
	}
	if v.Description != "" {
		meta("name", "description", v.Description)
	}
	if v.Canonical != "" {
		tags.WriteString(`<link rel="canonical" href="` + html.EscapeString(v.Canonical) + `">`)
		for _, pair := range [][2]string{
			{"og:type", "website"}, {"og:site_name", "gusto"}, {"og:title", v.Title}, {"og:description", v.Description},
			{"og:url", v.Canonical}, {"og:image", v.Share}, {"og:image:width", "1200"}, {"og:image:height", "630"},
			{"og:image:alt", "gusto: run agents across your team with control and compliance built in"},
		} {
			if pair[1] != "" {
				meta("property", pair[0], pair[1])
			}
		}
		for _, pair := range [][2]string{
			{"twitter:card", "summary_large_image"}, {"twitter:title", v.Title}, {"twitter:description", v.Description}, {"twitter:image", v.Share},
		} {
			if pair[1] != "" {
				meta("name", pair[0], pair[1])
			}
		}
	}
	if v.LD != "" {
		tags.WriteString(`<script type="application/ld+json">` + string(v.LD) + `</script>`)
	}
	return tags.String()
}
