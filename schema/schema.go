// Package schema holds the published formats of job.json and manifest.json, and
// the validator the tests check real outputs with.
//
// The files are the contract for anybody who writes a runner or a reader
// without reading the Go. Embedding them here means the tests validate against
// the very bytes that are published, so a writer that drifts from the schema
// fails a test rather than a stranger's integration.
package schema

import _ "embed"

//go:embed job/v1.json
var job []byte

//go:embed manifest/v1.json
var manifest []byte

// Job is the schema of job.json, version 1.
func Job() []byte { return job }

// Manifest is the schema of out/manifest.json, version 1.
func Manifest() []byte { return manifest }
