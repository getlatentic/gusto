package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
)

const (
	databaseURLEnv = "GUSTO_DATABASE_URL"
	// schemaLimit is Postgres's longest identifier, in bytes.
	schemaLimit = 63
)

// usageSchema is the Postgres schema an organisation's index of model calls lives in: org_ and the slug, with every
// character outside a-z, 0-9 and underscore made an underscore. A slug that was changed or is too long ends in
// a short hash of the slug itself, so that two slugs never share a schema.
func usageSchema(slug string) string {
	var body strings.Builder
	changed := false
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			body.WriteRune(r)
		default:
			body.WriteByte('_')
			changed = true
		}
	}
	name := "org_" + body.String()
	if !changed && len(name) <= schemaLimit {
		return name
	}
	sum := sha256.Sum256([]byte(slug))
	suffix := "_" + hex.EncodeToString(sum[:4])
	return truncated(name, schemaLimit-len(suffix)) + suffix
}

func truncated(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// indexFile is the name of an organisation's index in its own directory under the index root.
const indexFile = "index.db"

// usageDatabase is the GUSTO_DATABASE_URL an organisation's server is given, from the operator's settings. A Postgres
// address gets the search_path of the organisation's own schema, in place of any the operator wrote. Any other
// address, and none, gives the organisation a SQLite file of its own under indexDir, which is on the local disk of the
// host the server runs on: the index is built from the organisation's files and is rebuilt when it is lost, so it does
// not belong on a disk that hosts share.
func usageDatabase(operator []string, slug, indexDir string) (string, error) {
	var raw string
	for _, setting := range operator {
		if value, found := strings.CutPrefix(setting, databaseURLEnv+"="); found {
			raw = value
		}
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", badURL(raw, err)
	}
	if scheme := strings.ToLower(parsed.Scheme); scheme != "postgres" && scheme != "postgresql" {
		return localIndex(indexDir, slug)
	}
	query := parsed.Query()
	query.Set("search_path", usageSchema(slug))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// localIndex is the sqlite:// address of an organisation's index under indexDir, which the server's database kit opens
// as a file.
func localIndex(indexDir, slug string) (string, error) {
	dir, err := filepath.Abs(indexDir)
	if err != nil {
		return "", err
	}
	path := filepath.ToSlash(filepath.Join(dir, slug, indexFile))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "sqlite", Path: path}).String(), nil
}

// badURL is no error for an unset address, and a refusal that quotes nothing of an address that does not parse.
func badURL(raw string, err error) error {
	if raw == "" || err == nil {
		return nil
	}
	return errors.New(databaseURLEnv + " is not a valid URL")
}

// withoutSetting is the settings with every one of that name left out.
func withoutSetting(settings []string, name string) []string {
	kept := make([]string, 0, len(settings))
	for _, setting := range settings {
		if !strings.HasPrefix(setting, name+"=") {
			kept = append(kept, setting)
		}
	}
	return kept
}
