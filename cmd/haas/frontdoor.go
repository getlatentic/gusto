package main

import (
	"errors"
	"net/url"
	"os"

	"github.com/getlatentic/haas/internal/web"
)

// frontDoorFromEnv is what makes this server one organisation's in a hosted haas:
// HAAS_ENTRY_KEY is the file with the key the front door and this server share,
// HAAS_ORG is which organisation this is, and HAAS_FRONT_DOOR is where the front
// door is. All three or none: a server that has some is one started wrongly, and
// a server that did not notice would ask for a password nobody has.
func frontDoorFromEnv() (*web.FrontDoor, error) {
	keyFile, org, address := os.Getenv("HAAS_ENTRY_KEY"), os.Getenv("HAAS_ORG"), os.Getenv("HAAS_FRONT_DOOR")
	if keyFile == "" && org == "" && address == "" {
		return nil, nil
	}
	if keyFile == "" || org == "" || address == "" {
		return nil, errors.New("HAAS_ENTRY_KEY, HAAS_ORG and HAAS_FRONT_DOOR are set together or not at all")
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	if len(key) < 32 {
		return nil, errors.New("the key in " + keyFile + " is too short to sign anything with")
	}
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("HAAS_FRONT_DOOR is the front door's address, as https://cloud.example.com")
	}
	return &web.FrontDoor{Key: key, Org: org, URL: parsed.Scheme + "://" + parsed.Host}, nil
}
