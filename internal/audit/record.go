// Package audit turns one job directory into an export an auditor can read and
// check without asking an engineer: who asked, what ran, what changed, which
// model, how much it spent, and who approved it.
//
// An export is a job directory itself. It carries job.json and out/ exactly as
// the run left them, so the manifest check that verifies a job verifies the
// export with no second implementation. Beside them it carries audit.json, the
// summary, which is derived only from the files in the export, so a reader can
// derive it again and see that it says nothing the files do not.
package audit

// Version is the version of audit.json this code writes and the newest it
// reads. The schema that describes it is schema/audit/v2.json. Version 1 said
// the manifest's signer claim where version 2 says what a keyring checked, and
// a version 1 export is still read, and verified, as version 1.
const Version = 2

// The names an export's files have inside it.
const (
	SummaryFile  = "audit.json"
	StateFile    = "state.json"
	RequestsFile = "requests.jsonl"
	ManifestFile = "out/manifest.json"
	SpecFile     = "job.json"
)

// Record is audit.json.
type Record struct {
	Version    int         `json:"version"`
	Job        string      `json:"job"`
	ExportedAt int64       `json:"exportedAt"`
	Asked      Asked       `json:"asked"`
	Ran        Ran         `json:"ran"`
	Outcome    Outcome     `json:"outcome"`
	Changed    Changed     `json:"changed"`
	Model      Model       `json:"model"`
	Spend      Spend       `json:"spend"`
	Outbound   Outbound    `json:"outbound"`
	Approval   Approval    `json:"approval"`
	Requests   Fact[[]any] `json:"requests"`
	Evidence   Evidence    `json:"evidence"`
	Carried    []Carried   `json:"carried"`
	Redactions []Redaction `json:"redactions"`
	Policy     Policy      `json:"policy"`
}

// Asked is who asked for the job. The server knows how a job arrived and not
// who sent it, because everybody signs in with one shared password.
type Asked struct {
	Who Fact[string] `json:"who"`
	How Fact[any]    `json:"how"`
	At  Fact[any]    `json:"at"`
}

// Ran is job.json as the export carries it, and the harness the manifest says
// actually ran. The spec is kept as written rather than read through defaults,
// so a redacted field stays visibly redacted instead of failing a type.
type Ran struct {
	Spec            Fact[any]    `json:"spec"`
	Harness         string       `json:"harness"`
	HarnessExit     int          `json:"harnessExit"`
	StoppedAtBudget bool         `json:"stoppedAtBudget"`
	Session         Fact[string] `json:"session"`
}

// Outcome is what the gate decided and the state the server holds for the job.
type Outcome struct {
	State     Fact[any]     `json:"state"`
	Gate      Fact[Verdict] `json:"gate"`
	StartedAt Fact[any]     `json:"startedAt"`
	EndedAt   Fact[any]     `json:"endedAt"`
	// FinishedAt is when the manifest was written, which is when the run ended.
	FinishedAt int64 `json:"finishedAt"`
}

// Verdict is the gate's answer as the manifest records it.
type Verdict struct {
	Command []string `json:"command"`
	Code    int      `json:"code"`
	Passed  bool     `json:"passed"`
}

// Changed is what the run changed, read off the patch it left.
type Changed struct {
	Patch  Fact[string]   `json:"patch"`
	SHA256 Fact[string]   `json:"sha256"`
	Files  Fact[[]string] `json:"files"`
}

// Model is the model the job asked for, where the harness said it answered,
// and which credential the proxy added on the way out.
type Model struct {
	Asked       Fact[any]      `json:"asked"`
	Endpoint    string         `json:"endpoint"`
	Credentials Fact[[]string] `json:"credentials"`
}

// Spend holds two counts that are deliberately kept apart: what the harness
// reported, which is a claim, and what the proxy measured of the model calls
// that left the box. A call a spending cap refused never left, so it is
// counted apart and costs nothing.
type Spend struct {
	Tokens       Fact[int64]   `json:"tokens"`
	CostUSD      Fact[float64] `json:"costUsd"`
	ProxyCalls   Fact[int]     `json:"proxyCalls"`
	ProxyBytes   Fact[int64]   `json:"proxyBytes"`
	ProxyTokens  Fact[int64]   `json:"proxyTokens"`
	ProxyUSD     Fact[float64] `json:"proxyUsd"`
	RefusedCalls Fact[int]     `json:"refusedCalls"`
}

// Outbound is the traffic to hosts other than the model, as host:port. It
// costs no model money, and a host that was denied was never reached.
type Outbound struct {
	Reached Fact[[]string] `json:"reached"`
	Denied  Fact[[]string] `json:"denied"`
}

// Approval is who approved the change. An agent opens a pull request and a
// person merges it somewhere this platform does not see.
type Approval struct {
	By  Fact[string] `json:"by"`
	Why string       `json:"why"`
}

// Evidence is the manifest the export verifies against. Signer is the name the
// exporter's keyring gives the key that signed it, or evidence.SignedBy's words
// for a record no keyring could vouch for; never the manifest's own claim.
type Evidence struct {
	Manifest string `json:"manifest"`
	Signer   string `json:"signer"`
	Files    int    `json:"files"`
}

// Carried is one file in the export and its hash as exported. Covered says the
// run's own manifest vouches for it; a file it does not cover is the server's
// record, which the export vouches for alone.
type Carried struct {
	Name    string `json:"name"`
	SHA256  string `json:"sha256"`
	Covered bool   `json:"covered"`
}

// Redaction is one file the policy changed, and why. Original is the hash the
// manifest holds for it, so a person with the unredacted job can still match
// the file to this run.
type Redaction struct {
	File     string   `json:"file"`
	Action   string   `json:"action"`
	Rules    []string `json:"rules"`
	Original string   `json:"original"`
}

// The two things a policy does to a file.
const (
	Dropped   = "dropped"
	Rewritten = "rewritten"
)
