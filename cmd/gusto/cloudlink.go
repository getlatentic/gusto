package main

import (
	"errors"
	"net/url"
	"os"

	"github.com/getlatentic/gusto/internal/web"
)

// cloudOf is what makes this server one organization's in a hosted gusto: GUSTO_ENTRY_KEY is the file with the key gusto
// cloud and this server share, GUSTO_ORG is which organization this is, and GUSTO_CLOUD_URL is where gusto cloud is. Those three
// are set together or not at all: a server that has some is one started wrongly, and a server that did not notice would ask for
// a password nobody has. GUSTO_ORG_NAME is what the organization is called, which its pages show in place of its address.
func cloudOf(access accessSettings, org string) (*web.Cloud, error) {
	switch setOf(access.entryKey, org, access.cloudURL) {
	case 0:
		return nil, nil
	case 3:
	default:
		return nil, errors.New("GUSTO_ENTRY_KEY, GUSTO_ORG and GUSTO_CLOUD_URL are set together or not at all")
	}
	key, err := entryKey(access.entryKey)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(access.cloudURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("GUSTO_CLOUD_URL is gusto cloud's address, as https://cloud.example.com")
	}
	return &web.Cloud{Key: key, Org: org, Name: access.orgName, URL: parsed.Scheme + "://" + parsed.Host}, nil
}

// setOf is how many of values say something.
func setOf(values ...string) int {
	set := 0
	for _, each := range values {
		if each != "" {
			set++
		}
	}
	return set
}

// entryKey is the key in file, long enough to sign an entry with.
func entryKey(file string) ([]byte, error) {
	key, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if len(key) < 32 {
		return nil, errors.New("the key in " + file + " is too short to sign anything with")
	}
	return key, nil
}
