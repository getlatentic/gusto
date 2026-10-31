package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/getlatentic/gusto/internal/connector"
)

// openConnectors is the store of people's sign-ins to apps, when GUSTO_CONNECTORS names its folder, and nil when it names none.
// GUSTO_PUBLIC_URL is this server's address, which a sign-in at an app comes back to. GUSTO_CONNECTOR_CLIENTS is a JSON file of
// the clients the operator registered with the apps that need one, by connector id: {"github": {"id": "…", "secret": "…"}}.
// GUSTO_CONNECTOR_APPS is a JSON file of more apps than the catalog's, such as the organization's own: [{"id", "name", "what", "url"}].
func openConnectors() (*connector.Connectors, error) {
	dir := os.Getenv("GUSTO_CONNECTORS")
	if dir == "" {
		return nil, nil
	}
	base := os.Getenv("GUSTO_PUBLIC_URL")
	if base == "" {
		return nil, errors.New("GUSTO_CONNECTORS needs GUSTO_PUBLIC_URL, the address a sign-in at an app comes back to")
	}
	var clients map[string]connector.Client
	if err := readSettingsFile(os.Getenv("GUSTO_CONNECTOR_CLIENTS"), &clients); err != nil {
		return nil, err
	}
	var more []connector.Entry
	if err := readSettingsFile(os.Getenv("GUSTO_CONNECTOR_APPS"), &more); err != nil {
		return nil, err
	}
	apps, err := connector.WithMore(more)
	if err != nil {
		return nil, err
	}
	return connector.Open(connector.Config{Dir: dir, Base: base, Clients: clients, Apps: apps, Private: os.Getenv("GUSTO_GATEWAY_PRIVATE") == "1"})
}

// readSettingsFile reads a JSON file into a value, and leaves it as it is when no file is named.
func readSettingsFile(path string, into any) error {
	if path == "" {
		return nil
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(text, into); err != nil {
		return fmt.Errorf("%s is not valid: %w", path, err)
	}
	return nil
}
