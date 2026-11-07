package cloud

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/getlatentic/gusto/internal/signing"
)

// prepare makes what an organization's server keeps: its directories, the key it signs
// its records with, and the key it and gusto cloud share.
func (c *Controller) prepare(ctx context.Context, slug string) error {
	data := c.Store.DataDir(slug)
	for _, dir := range []string{"jobs", "sessions", "setups", "comparisons", "changes", "retention", "keys", "computers", "inbox"} {
		if err := os.MkdirAll(filepath.Join(data, dir), 0o700); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(c.orgDir(slug), "home"), 0o700); err != nil {
		return err
	}
	if err := c.Store.PublishRoster(ctx, slug); err != nil {
		return err
	}
	keys := filepath.Join(data, "keys")
	if _, err := os.Stat(filepath.Join(keys, slug+".key")); missing(err) {
		if _, err := signing.Generate(keys, slug); err != nil {
			return err
		}
	}
	if _, err := os.Stat(c.entryKeyFile(slug)); missing(err) {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		return os.WriteFile(c.entryKeyFile(slug), []byte(hex.EncodeToString(secret)), 0o600)
	}
	return nil
}

// entryKeyFile is the key gusto cloud makes entry tokens with and the
// organization's server checks them with.
func (c *Controller) entryKeyFile(slug string) string {
	return filepath.Join(c.orgDir(slug), "entry.key")
}

// EntryKey is that key.
func (c *Controller) EntryKey(slug string) ([]byte, error) { return os.ReadFile(c.entryKeyFile(slug)) }

// environment is the whole environment of an organization's server: the operator's
// settings, then what is the organization's own, which wins.
func (c *Controller) environment(ctx context.Context, slug string, runtime Runtime, node Node) ([]string, error) {
	password := make([]byte, 24)
	if _, err := rand.Read(password); err != nil {
		return nil, err
	}
	org, _, err := c.Store.Org(ctx, slug)
	if err != nil {
		return nil, err
	}
	own := c.dataSettings(slug)
	maps.Copy(own, c.serverSettings(slug, org.Name, runtime, node))
	maps.Copy(own, map[string]string{
		// A person's own conversations share their computer's files and their own state, so they
		// take turns. A room's agents each work on a copy, so they do not wait for each other.
		"GUSTO_LIMIT_PERSON":     "1",
		"GUSTO_TASK_CONCURRENCY": strconv.Itoa(max(c.Concurrent, 1)),
		// Nobody knows this: a server behind gusto cloud has no password to type.
		"GUSTO_PASSWORD": hex.EncodeToString(password),
		"HOME":           filepath.Join(c.orgDir(slug), "home"),
	})
	data := c.Store.DataDir(slug)
	if c.OrgURL != nil {
		own["GUSTO_PUBLIC_URL"] = c.OrgURL(slug)
		own["GUSTO_CONNECTORS"] = filepath.Join(data, "connectors")
	}
	operator := c.Env
	if org.Demo != nil {
		operator = append(slices.Clone(c.Env), c.DemoEnv...)
	}
	database, err := usageDatabase(operator, slug, c.indexDir())
	if err != nil {
		return nil, err
	}
	if database != "" {
		own[databaseURLEnv] = database
	}
	env := withoutSetting(operator, databaseURLEnv)
	for name, value := range own {
		env = append(env, name+"="+value)
	}
	return env, nil
}

// serverSettings is where an organization's server listens, which organization it is, and the gusto cloud and node it is on.
func (c *Controller) serverSettings(slug, name string, runtime Runtime, node Node) map[string]string {
	return map[string]string{
		"GUSTO_ADDR":       node.Listen() + ":" + strconv.Itoa(runtime.Port),
		"GUSTO_PROXY_ADDR": "127.0.0.1:" + strconv.Itoa(runtime.ProxyPort),
		"GUSTO_ORG":        slug,
		"GUSTO_ORG_NAME":   name,
		"GUSTO_CLOUD_URL":  c.CloudURL,
		"GUSTO_NODE":       node.Name(),
	}
}

// dataSettings is where an organization's server keeps each kind of its data, all under its own directory.
func (c *Controller) dataSettings(slug string) map[string]string {
	data := c.Store.DataDir(slug)
	return map[string]string{
		"GUSTO_TASKS":        filepath.Join(data, "jobs"),
		"GUSTO_SESSIONS":     filepath.Join(data, "sessions"),
		"GUSTO_SETUPS":       filepath.Join(data, "setups"),
		"GUSTO_CREW":         filepath.Join(data, "crew"),
		"GUSTO_PRESETS":      filepath.Join(data, "presets"),
		"GUSTO_INTEGRATIONS": filepath.Join(data, "integrations"),
		"GUSTO_COMPARISONS":  filepath.Join(data, "comparisons"),
		"GUSTO_CHANGES":      filepath.Join(data, "changes", "changes.jsonl"),
		"GUSTO_RETENTION":    filepath.Join(data, "retention"),
		signing.KeyEnv:       filepath.Join(data, "keys", slug+".key"),
		signing.KeyringEnv:   filepath.Join(data, "keys"),
		"GUSTO_SIGNER":       slug,
		"GUSTO_QUOTA":        c.Store.QuotaFile(slug),
		"GUSTO_MEMBERS":      c.Store.RosterFile(slug),
		"GUSTO_ORG_SETTINGS": filepath.Join(data, "settings.json"),
		"GUSTO_REGISTRY":     filepath.Join(data, "registry"),
		"GUSTO_INBOX":        c.Store.InboxDir(slug),
		"GUSTO_ENTRY_KEY":    c.entryKeyFile(slug),
		"GUSTO_COMPUTERS":    filepath.Join(data, "computers"),
		"GUSTO_APPS":         filepath.Join(data, "apps"),
	}
}
