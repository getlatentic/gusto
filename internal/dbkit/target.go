package dbkit

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

var schemaName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]{0,62}$`)

// target is where a database lives, with every secret kept apart so that no message can carry one.
type target struct {
	dialect Dialect
	path    string
	pg      *pgx.ConnConfig
	schema  string
	logURL  string
	secrets []string
}

func acceptedForms(file string) string {
	return "use postgres://user:password@host/database, postgresql://..., sqlite:///path/to/file.db, or leave it empty for " +
		file + " in the data directory"
}

func parseTarget(databaseURL, dir, file string) (target, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return sqliteTarget(filepath.Join(dir, file)), nil
	}
	u, err := url.Parse(databaseURL)
	if err != nil {
		return target{}, fmt.Errorf("dbkit: the database URL is not valid; %s", acceptedForms(file))
	}
	switch strings.ToLower(u.Scheme) {
	case "postgres", "postgresql":
		return postgresTarget(u, file)
	case "sqlite":
		return sqliteURLTarget(u, dir, file), nil
	}
	return target{}, fmt.Errorf("dbkit: database URL scheme %q is not supported; %s", u.Scheme, acceptedForms(file))
}

func sqliteTarget(path string) target {
	return target{dialect: SQLite, path: path, logURL: "sqlite://" + filepath.ToSlash(path)}
}

func sqliteURLTarget(u *url.URL, dir, file string) target {
	path := u.Host + u.Path
	if len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	if path == "" {
		path = filepath.Join(dir, file)
	}
	return sqliteTarget(filepath.FromSlash(path))
}

func postgresTarget(u *url.URL, file string) (target, error) {
	q := u.Query()
	schema := q.Get("search_path")
	q.Del("search_path")
	if schema != "" && !schemaName.MatchString(schema) {
		return target{}, errors.New("dbkit: search_path must name one schema, of letters, digits and underscores")
	}
	clean := *u
	clean.RawQuery = q.Encode()
	cfg, err := pgx.ParseConfig(clean.String())
	if err != nil {
		return target{}, fmt.Errorf("dbkit: the database URL is not valid; %s", acceptedForms(file))
	}
	if schema != "" {
		cfg.RuntimeParams["search_path"] = schema
	}
	return target{dialect: Postgres, pg: cfg, schema: schema, logURL: withoutPassword(u), secrets: secretsOf(u, cfg)}, nil
}

func secretsOf(u *url.URL, cfg *pgx.ConnConfig) []string {
	plain := []string{cfg.Password}
	if p, ok := u.User.Password(); ok {
		plain = append(plain, p)
	}
	for _, key := range []string{"password", "sslpassword"} {
		plain = append(plain, u.Query().Get(key))
	}
	var out []string
	for _, p := range plain {
		if p != "" {
			out = append(out, p, url.QueryEscape(p), url.PathEscape(p))
		}
	}
	slices.SortFunc(out, func(a, b string) int { return len(b) - len(a) })
	return slices.Compact(out)
}

func withoutPassword(u *url.URL) string {
	c := *u
	if c.User != nil {
		c.User = url.User(c.User.Username())
	}
	q := c.Query()
	q.Del("password")
	q.Del("sslpassword")
	c.RawQuery = q.Encode()
	return c.String()
}

// redact removes every secret from an error, since a driver may quote the connection it was given.
func (t target) redact(err error) error {
	if err == nil || len(t.secrets) == 0 {
		return err
	}
	msg := err.Error()
	for _, s := range t.secrets {
		msg = strings.ReplaceAll(msg, s, "xxxxx")
	}
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}
