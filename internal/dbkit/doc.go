// Package dbkit is the database layer that gusto's components share: it opens SQLite or Postgres from one
// URL, runs each component's own embedded migrations, retries transactions that lost a race, and holds the
// few places where the two databases differ.
//
// Queries are written once with ? placeholders and standard SQL; DB.Rebind, the Querier a DB or Tx offers,
// and the helpers in this package cover the differences.
package dbkit
