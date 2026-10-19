// Package evidence is what a task leaves behind, and the hash of all of it.
//
// This is what makes a run worth anything to somebody who was not there. A
// buyer, an auditor or a marketplace re-hashes the files and knows whether they
// are the ones the run produced.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/getlatentic/gusto/internal/job"
)

// Verdict is the check's answer. Nothing else in a run has one to give.
type Verdict struct {
	Command []string `json:"command"`
	Code    int      `json:"code"`
	Passed  bool     `json:"passed"`
}

// CapStop says which spending cap stopped a run: the proxy refused the
// agent's calls to a model once it was spent. Reason is written for the
// person who asked for the task.
type CapStop struct {
	// Cap is "job" (a per-task cap) or "team".
	Cap      string  `json:"cap"`
	Team     string  `json:"team,omitempty"`
	LimitUSD float64 `json:"limitUsd"`
	SpentUSD float64 `json:"spentUsd"`
	Reason   string  `json:"reason"`
}

// Entry is one file and its hash.
type Entry struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// Manifest is the record of a run. A token count or a cost is a pointer, so a
// agent that does not report one leaves null rather than a zero somebody
// would read as free.
//
// Version is the format of manifest.json. A file without one is version 1.
type Manifest struct {
	Version int     `json:"version"`
	Job     string  `json:"job"`
	Files   []Entry `json:"files"`
	// Gate is absent when the task had none to run: a turn in a conversation is
	// not a claim that anything passed.
	Gate    *Verdict `json:"gate,omitempty"`
	Harness string   `json:"harness"`
	// HarnessDigest names the configured agent that ran by what it held,
	// taken before it started. A name is what two teams can both say and mean
	// different things by; this is not. It is empty in a manifest written
	// before it was recorded, which says so rather than guessing one.
	HarnessDigest string `json:"harnessDigest,omitempty"`
	// Session is what the agent calls the conversation this run was a turn in.
	Session         string `json:"session,omitempty"`
	HarnessExit     int    `json:"harnessExit"`
	StoppedAtBudget bool   `json:"stoppedAtBudget"`
	// StoppedBy is absent unless the run was told to stop: by the person who stopped it, or by "a signal".
	StoppedBy string `json:"stoppedBy,omitempty"`
	// StoppedByCap is absent unless a spending cap refused the run a call.
	StoppedByCap  *CapStop `json:"stoppedByCap,omitempty"`
	ModelEndpoint string   `json:"modelEndpoint"`
	Tokens        *int64   `json:"tokens"`
	CostUSD       *float64 `json:"costUsd"`
	// Signer is who the runner says stands behind this run. It is a claim; the
	// signer a verifier names comes from its keyring, through Attest.
	Signer string `json:"signer"`
	AtUnix int64  `json:"atUnix"`
}

// Version is the version of manifest.json this code writes and the newest it
// reads. The schema that describes it is schema/manifest/v1.json.
const Version = 1

func Now() int64 { return time.Now().Unix() }

// Of hashes the task spec and every file in out/ except the manifest itself and
// its signature, which is made over the manifest after it is written.
func Of(dir job.Dir) (string, []Entry, error) {
	spec, err := hash(dir.Spec())
	if err != nil {
		return "", nil, err
	}
	listing, err := os.ReadDir(dir.Out())
	if err != nil {
		return "", nil, err
	}
	files := make([]Entry, 0, len(listing))
	for _, entry := range listing {
		// A link, a pipe or a device in out/ is not a file the record can vouch
		// for: hashing a link would read what it points at, and a pipe would
		// never end. Check names it.
		if !entry.Type().IsRegular() || entry.Name() == "manifest.json" || entry.Name() == SignatureFile {
			continue
		}
		sum, err := hash(dir.OutFile(entry.Name()))
		if err != nil {
			return "", nil, err
		}
		files = append(files, Entry{Name: entry.Name(), SHA256: sum})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return spec, files, nil
}

// Write keeps the manifest, stamping the version this build writes when the
// caller left it out. A signature already beside it signed other bytes, so it
// goes: a rewritten record is unsigned until somebody holding a key signs it.
func Write(dir job.Dir, manifest Manifest) error {
	if manifest.Version == 0 {
		manifest.Version = Version
	}
	text, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.Remove(dir.OutFile(SignatureFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(dir.OutFile("manifest.json"), append(text, '\n'), 0o644)
}

// Read the manifest. A field this build does not know is ignored, and a version
// it does not know is refused, for the reasons job.Readable gives.
func Read(dir job.Dir) (Manifest, error) {
	text, err := os.ReadFile(dir.OutFile("manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(text, &manifest); err != nil {
		return Manifest{}, err
	}
	if err := job.Readable("manifest.json", manifest.Version, Version); err != nil {
		return Manifest{}, err
	}
	if manifest.Version == 0 {
		manifest.Version = Version
	}
	return manifest, nil
}

// Check reports every file whose hash no longer matches the manifest, every one
// the manifest names that is gone, every one that appeared since, and a
// signature that no longer holds for the manifest. Who signed is Attest's
// question, because it needs a keyring and this check does not.
func Check(dir job.Dir) ([]string, error) {
	manifest, err := Read(dir)
	if err != nil {
		return nil, err
	}
	spec, files, err := Of(dir)
	if err != nil {
		return nil, err
	}
	wrong := append(sealComplaints(dir), harnessComplaints(dir, manifest)...)
	if spec != manifest.Job {
		wrong = append(wrong, "job.json changed")
	}
	found := map[string]string{}
	for _, entry := range files {
		found[entry.Name] = entry.SHA256
	}
	for _, entry := range manifest.Files {
		switch sum, ok := found[entry.Name]; {
		case !ok:
			wrong = append(wrong, fmt.Sprintf("%s is missing", entry.Name))
		case sum != entry.SHA256:
			wrong = append(wrong, fmt.Sprintf("%s changed", entry.Name))
		}
	}
	if listing, err := os.ReadDir(dir.Out()); err == nil {
		for _, entry := range listing {
			if !entry.IsDir() && !entry.Type().IsRegular() {
				wrong = append(wrong, fmt.Sprintf("%s is not a plain file", entry.Name()))
			}
		}
	}
	named := map[string]bool{}
	for _, entry := range manifest.Files {
		named[entry.Name] = true
	}
	for _, entry := range files {
		if !named[entry.Name] {
			wrong = append(wrong, fmt.Sprintf("%s is not in the manifest", entry.Name))
		}
	}
	return wrong, nil
}

func hash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
