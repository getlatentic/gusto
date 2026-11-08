package cloud

import (
	"net/url"
	"strings"
)

// bookingFrames are the booking services whose pages may be framed, by host: the origins the page's policy must allow, since a
// short link of Google's is answered from its calendar's own host, and the settings that ask for the page's embedded form.
var bookingFrames = map[string]struct {
	origins []string
	embed   map[string]string
}{
	"calendly.com":        {origins: []string{"https://calendly.com"}, embed: map[string]string{"embed_type": "Inline"}},
	"calendar.google.com": {origins: []string{"https://calendar.google.com"}, embed: map[string]string{"gv": "true"}},
	"calendar.app.google": {origins: []string{"https://calendar.app.google", "https://calendar.google.com"}},
}

// bookingFrame is the address to frame a booking page from, and the origins the page must allow to frame it, for a booking address
// of a service that may be framed. Any other address, and one that is not https, is linked to and not framed.
func bookingFrame(address string) (frame string, origins []string) {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return "", nil
	}
	service, known := bookingFrames[strings.ToLower(parsed.Hostname())]
	if !known || parsed.Port() != "" {
		return "", nil
	}
	if len(service.embed) > 0 {
		query := parsed.Query()
		for name, value := range service.embed {
			query.Set(name, value)
		}
		parsed.RawQuery = query.Encode()
	}
	return parsed.String(), service.origins
}
