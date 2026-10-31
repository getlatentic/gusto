package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/getlatentic/gusto/internal/connector"
)

// openConnectors is the store of people's sign-ins to apps, when GUSTO_CONNECTORS names its folder, and nil when it names none.
// GUSTO_PUBLIC_URL is this server's address, which a sign-in at an app comes back to. GUSTO_CONNECTOR_CLIENTS is a JSON file of
// the clients the operator registered with the apps that need one, by connector id: {"github": {"id": "…", "secret": "…"}}, or
// ssm:<name>, a SecureString parameter holding the same.
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
	clients, err := connectorClients(context.Background(), os.Getenv("GUSTO_CONNECTOR_CLIENTS"), openParameters)
	if err != nil {
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

// connectorClients reads the operator's clients from a file, or from Parameter Store when the setting is ssm:<name>. A parameter
// that cannot be read leaves every app that needs a client off, and is said, so a server that hosts an organization still starts.
func connectorClients(ctx context.Context, setting string, open func(context.Context) (parameterReader, error)) (map[string]connector.Client, error) {
	var clients map[string]connector.Client
	name, fromParameter := strings.CutPrefix(setting, "ssm:")
	if !fromParameter {
		return clients, readSettingsFile(setting, &clients)
	}
	text, err := readParameter(ctx, open, name)
	if err == nil {
		err = json.Unmarshal([]byte(text), &clients)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "gusto: no app that needs a client can be connected, since the clients in %s could not be read: %v\n", name, err)
		return nil, nil
	}
	return clients, nil
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
