package openshell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/getlatentic/gusto/pkg/harness"
)

// A blank computer is made ahead of the person it will be for. The gateway names a sandbox when it makes it and has no call
// to rename one, and it changes only a live sandbox's network rules, which the sandbox takes up at its next poll of the
// gateway, ten seconds by default. So what a person's computer is made from has to be there at the start: the image, the
// limits and the whole policy. Nothing else of a person is in a computer when it is made. The workspace, the environment and
// the token go in when a task starts, and the person's record names the sandbox, whatever it is called.

// removeWithin is how long the gateway has to remove a sandbox.
const removeWithin = 2 * time.Minute

// ComputerPolicy is the policy of a computer for an agent, which is made when the computer is. Two agents whose own
// folders are in the home get the same policy.
func ComputerPolicy(cfg Config, modelURL string, adapter harness.Adapter) ([]byte, error) {
	var keeps []string
	if adapter.Keeps != "" {
		keeps = append(keeps, harness.Rehome(adapter.Keeps, Home))
	}
	return Policy(cfg, modelURL, keeps)
}

// PolicyKey says which policies are the same: a computer made ahead is for a person whose policy has this key.
func PolicyKey(policy []byte) string {
	sum := sha256.Sum256(policy)
	return hex.EncodeToString(sum[:])
}

// BlankKey is the key of the computers MakeBlank makes, which is the key of a person's computer when it is made the same way.
func BlankKey(cfg Config, modelURL string) (string, error) {
	policy, err := ComputerPolicy(cfg, modelURL, harness.Adapter{})
	if err != nil {
		return "", err
	}
	return PolicyKey(policy), nil
}

// MakeBlank makes the sandbox called name as a person's computer is made, from the same image, with the same limits and the
// same policy, and nothing in it. What the person owns arrives with their first task.
func MakeBlank(ctx context.Context, cfg Config, name, modelURL string) error {
	policy, err := ComputerPolicy(cfg, modelURL, harness.Adapter{})
	if err != nil {
		return err
	}
	return (&Sandbox{cfg: cfg, name: name, model: modelURL, policy: policy}).makeSandboxOnly(ctx)
}

// Remove removes a sandbox by its name, and one that is not there is removed.
func Remove(ctx context.Context, cfg Config, name string) error {
	if _, err := output((&Sandbox{cfg: cfg}).run(ctx, "sandbox", "delete", name)); err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}
	return nil
}

// listPageSize is the most names asked of the gateway at once, well above the sandboxes one host holds.
const listPageSize = "1000"

// Named lists the sandboxes on the gateway whose names begin with prefix.
func Named(ctx context.Context, cfg Config, prefix string) ([]string, error) {
	listed, err := output((&Sandbox{cfg: cfg}).run(ctx, "sandbox", "list", "--names", "--page-size", listPageSize))
	if err != nil {
		return nil, err
	}
	var found []string
	for _, name := range strings.Fields(string(listed)) {
		if strings.HasPrefix(name, prefix) {
			found = append(found, name)
		}
	}
	return found, nil
}
