package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var linkInMail = regexp.MustCompile(`https?://\S+\?t=\S+`)

// linkFor is the newest link of a kind that the outbox holds for an address, which is what
// the person would have clicked, as the token in it.
func linkFor(outbox, to, path string) (string, error) {
	names, _ := filepath.Glob(filepath.Join(outbox, "*.eml"))
	sort.Strings(names)
	for i := len(names) - 1; i >= 0; i-- {
		text, err := os.ReadFile(names[i])
		if err != nil || !strings.Contains(string(text), "\r\nTo: "+to+"\r\n") {
			continue
		}
		for _, found := range linkInMail.FindAllString(string(text), -1) {
			parsed, err := url.Parse(found)
			if err == nil && parsed.Path == path {
				return parsed.Query().Get("t"), nil
			}
		}
	}
	return "", fmt.Errorf("no mail to %s holds a %s link", to, path)
}
