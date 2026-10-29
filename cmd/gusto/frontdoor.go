package main

import (
	"errors"
	"net/url"
	"os"

	"github.com/getlatentic/gusto/internal/web"
)

// frontDoorFromEnv is what makes this server one organization's in a hosted gusto:
// GUSTO_ENTRY_KEY is the file with the key gusto cloud and this server share,
// GUSTO_ORG is which organization this is, and GUSTO_FRONT_DOOR is where gusto
// cloud is. Those three are set together or not at all: a server that has some is
// one started wrongly, and a server that did not notice would ask for a password
// nobody has. GUSTO_ORG_NAME is what the organization is called, which its pages
// show in place of its address.
func frontDoorFromEnv() (*web.FrontDoor, error) {
	keyFile, org, address := os.Getenv("GUSTO_ENTRY_KEY"), os.Getenv("GUSTO_ORG"), os.Getenv("GUSTO_FRONT_DOOR")
	if keyFile == "" && org == "" && address == "" {
		return nil, nil
	}
	if keyFile == "" || org == "" || address == "" {
		return nil, errors.New("GUSTO_ENTRY_KEY, GUSTO_ORG and GUSTO_FRONT_DOOR are set together or not at all")
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
		return nil, errors.New("GUSTO_FRONT_DOOR is gusto cloud's address, as https://cloud.example.com")
	}
	return &web.FrontDoor{Key: key, Org: org, Name: os.Getenv("GUSTO_ORG_NAME"), URL: parsed.Scheme + "://" + parsed.Host}, nil
}
