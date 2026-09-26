package audit

import (
	"os"
	"path/filepath"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
)

// approvalUnseen is why an export cannot name an approver: the platform's part
// ends at the pull request, and the merge happens on the forge.
const approvalUnseen = "an agent opens a pull request and never merges; a person merges it on the forge, which this platform does not record"

// harnessDefault is what a job that names no model asked for.
const harnessDefault = "the harness's default"

// Summarize derives audit.json from the files in an export and nothing else.
// Export writes what this returns, and Verify calls it again and compares, so
// the summary can never say something its files do not.
func Summarize(dir job.Dir, id string, redactions []Redaction, policy Policy) (Record, error) {
	manifest, err := evidence.Read(dir)
	if err != nil {
		return Record{}, err
	}
	why := reasons(redactions)
	spec := document(dir.Spec(), why(SpecFile))
	state := document(filepath.Join(dir.Path(), StateFile), why(StateFile))
	carried, err := carriedIn(dir, manifest)
	if err != nil {
		return Record{}, err
	}
	return Record{
		Version:    Version,
		Job:        id,
		Asked:      askedOf(state),
		Ran:        ranOf(spec, manifest),
		Outcome:    outcomeOf(state, manifest),
		Changed:    changedOf(dir, manifest, why("out/diff.patch")),
		Model:      modelOf(spec, manifest, lines(dir.OutFile("proxy.jsonl"), why("out/proxy.jsonl"))),
		Spend:      spendOf(manifest, lines(dir.OutFile("proxy.jsonl"), why("out/proxy.jsonl"))),
		Approval:   Approval{By: absent[string](NotRecorded), Why: approvalUnseen},
		Requests:   lines(filepath.Join(dir.Path(), RequestsFile), why(RequestsFile)),
		Evidence:   Evidence{Manifest: ManifestFile, Signer: manifest.Signer, Files: len(manifest.Files)},
		Carried:    carried,
		Redactions: nonNil(redactions),
		Policy:     policy.written(),
	}, nil
}

// reasons says why a file is not in the export: the policy dropped it, or the
// platform never wrote it.
func reasons(redactions []Redaction) func(string) string {
	dropped := map[string]bool{}
	for _, redaction := range redactions {
		if redaction.Action == Dropped {
			dropped[redaction.File] = true
		}
	}
	return func(name string) string {
		if dropped[name] {
			return Redacted
		}
		return NotRecorded
	}
}

func document(file, why string) Fact[any] {
	value, err := readDocument(file)
	if err != nil {
		return absent[any](why)
	}
	return known(value)
}

func lines(file, why string) Fact[[]any] {
	value, err := readLines(file)
	if err != nil {
		return absent[[]any](why)
	}
	return known(value)
}

// field is one member of a document the export may not hold.
func field(document Fact[any], name string) Fact[any] {
	if document.absent != "" {
		return absent[any](document.absent)
	}
	value, found := member(document.value, name)
	if !found {
		return absent[any](NotRecorded)
	}
	return known(value)
}

// askedOf names how the job arrived. Who sent it is not recorded: everybody
// signs in with the same password, so the server has no name to write.
func askedOf(state Fact[any]) Asked {
	return Asked{
		Who: absent[string](NotRecorded),
		How: field(state, "asked"),
		At:  field(state, "queuedAt"),
	}
}

func ranOf(spec Fact[any], manifest evidence.Manifest) Ran {
	session := absent[string](None)
	if manifest.Session != "" {
		session = known(manifest.Session)
	}
	return Ran{
		Spec:            spec,
		Harness:         manifest.Harness,
		HarnessExit:     manifest.HarnessExit,
		StoppedAtBudget: manifest.StoppedAtBudget,
		Session:         session,
	}
}

func outcomeOf(state Fact[any], manifest evidence.Manifest) Outcome {
	gate := absent[Verdict](None)
	if manifest.Gate != nil {
		gate = known(Verdict(*manifest.Gate))
	}
	return Outcome{
		State:      field(state, "state"),
		Gate:       gate,
		StartedAt:  field(state, "startedAt"),
		EndedAt:    field(state, "endedAt"),
		FinishedAt: manifest.AtUnix,
	}
}

// changedOf reads the patch. Its hash is the one the manifest holds, which is
// the run's own, even when the policy rewrote the copy the export carries.
func changedOf(dir job.Dir, manifest evidence.Manifest, why string) Changed {
	sum, named := hashIn(manifest, "diff.patch")
	patch, err := os.ReadFile(dir.OutFile("diff.patch"))
	if !named || err != nil {
		if !named {
			why = NotRecorded
		}
		return Changed{Patch: absent[string](why), SHA256: absent[string](why), Files: absent[[]string](why)}
	}
	return Changed{Patch: known("out/diff.patch"), SHA256: known(sum), Files: known(changedFiles(patch))}
}

func modelOf(spec Fact[any], manifest evidence.Manifest, proxy Fact[[]any]) Model {
	asked := field(spec, "model")
	if asked.absent == NotRecorded {
		asked = known[any](harnessDefault)
	}
	credentials := absent[[]string](proxy.absent)
	if proxy.absent == "" {
		credentials = known(proxyCounts(proxy.value).credentials)
	}
	return Model{Asked: asked, Endpoint: manifest.ModelEndpoint, Credentials: credentials}
}

// spendOf keeps the harness's claim and the proxy's count side by side. The
// proxy counts calls and bytes, not tokens, so neither stands in for the other.
func spendOf(manifest evidence.Manifest, proxy Fact[[]any]) Spend {
	spend := Spend{
		Tokens:     absent[int64](NotRecorded),
		CostUSD:    absent[float64](NotRecorded),
		ProxyCalls: absent[int](proxy.absent),
		ProxyBytes: absent[int64](proxy.absent),
	}
	if manifest.Tokens != nil {
		spend.Tokens = known(*manifest.Tokens)
	}
	if manifest.CostUSD != nil {
		spend.CostUSD = known(*manifest.CostUSD)
	}
	if proxy.absent == "" {
		counts := proxyCounts(proxy.value)
		spend.ProxyCalls, spend.ProxyBytes = known(counts.calls), known(counts.bytes)
	}
	return spend
}

func hashIn(manifest evidence.Manifest, name string) (string, bool) {
	for _, entry := range manifest.Files {
		if entry.Name == name {
			return entry.SHA256, true
		}
	}
	return "", false
}

func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}
