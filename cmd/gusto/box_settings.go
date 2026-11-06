package main

import (
	"context"
	"os/exec"
	"strings"

	"github.com/getlatentic/gusto/internal/isolation"
	"github.com/getlatentic/gusto/internal/openshell"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/internal/settings"
	"github.com/getlatentic/gusto/internal/signing"
)

// signingSettings is who stands behind the records a command writes, and the keyring that names the signers of the records
// it reads.
type signingSettings struct {
	key, name, keyring string
}

func (s *signingSettings) declare(set *settings.Set) {
	s.declareSigner(set)
	s.declareKeyring(set)
}

func (s *signingSettings) declareSigner(set *settings.Set) {
	set.String(&s.key, signing.KeyEnv, "", "the private key records are signed with: a file from gusto keygen, or awskms://<key>")
	set.String(&s.name, signing.NameEnv, "", "the name a record gives for who signed it (default the key's id)")
}

func (s *signingSettings) declareKeyring(set *settings.Set) {
	set.String(&s.keyring, signing.KeyringEnv, "", "the directory of public keys that name a record's signer")
}

// identity opens the key, so a key that cannot be read stops the command before any task runs.
func (s signingSettings) identity() (signing.Identity, error) {
	return signing.Open(context.Background(), s.key, s.name)
}

func (s signingSettings) openKeyring() (signing.Keyring, error) {
	return signing.OpenKeyring(s.keyring)
}

// containerSettings is how a command starts the containers tasks and programs run in.
type containerSettings struct {
	c sandbox.Containers
}

func (s *containerSettings) declare(set *settings.Set) {
	set.String(&s.c.Runtime, "GUSTO_CONTAINER", "", "the container command, docker or podman (default docker, or podman when only it is installed)")
	set.String(&s.c.Image, "GUSTO_IMAGE", "gusto:dev", "the runner image tasks run in")
	set.String(&s.c.ImageSource, "GUSTO_IMAGE_SOURCE", "", "where the runner image is fetched from when this computer does not have it: s3:// or http(s)://")
	set.String(&s.c.CPUs, "GUSTO_JOB_CPUS", "1", "the CPUs one task's container may use")
	set.String(&s.c.Memory, "GUSTO_JOB_MEMORY", "3g", "the memory one task's container may use")
	set.String(&s.c.ToolImage, "GUSTO_TOOLS_IMAGE", "", "the image a tool server that is a program runs in (default GUSTO_IMAGE)")
	set.Choice(&s.c.ToolNetwork, "GUSTO_TOOLS_NETWORK", "", []string{"open"}, "open gives a tool server the runtime's network")
	set.String(&s.c.ToolCPUs, "GUSTO_TOOL_CPUS", "0.5", "the CPUs one tool server's container may use")
	set.String(&s.c.ToolMemory, "GUSTO_TOOL_MEMORY", "512m", "the memory one tool server's container may use")
}

// signedBy is the containers with the container command this computer has, whose records signs stands behind.
func (s containerSettings) signedBy(signs signing.Identity) sandbox.Containers {
	c := s.c
	c.Runtime = isolation.RuntimeName(c.Runtime, exec.LookPath)
	c.Signing = signs
	return c
}

// openShellSettings is where the OpenShell gateway is and what its sandboxes are made from.
type openShellSettings struct {
	cfg openshell.Config
}

func (s *openShellSettings) declare(set *settings.Set) {
	set.String(&s.cfg.Binary, "GUSTO_OPENSHELL", "openshell", "the openshell command")
	set.String(&s.cfg.Gateway, "GUSTO_OPENSHELL_GATEWAY", "", "the registered gateway to use (default the selected one)")
	set.String(&s.cfg.Image, "GUSTO_OPENSHELL_IMAGE", "", "what a sandbox is made from: an image the gateway can pull, or a root file system archive")
	set.String(&s.cfg.Host, "GUSTO_OPENSHELL_HOST", "host.openshell.internal", "the name a sandbox uses for the machine the gateway runs on")
	set.String(&s.cfg.CPU, "GUSTO_OPENSHELL_CPU", "1", "the CPUs one sandbox may use")
	set.String(&s.cfg.Memory, "GUSTO_OPENSHELL_MEMORY", "3Gi", "the memory one sandbox may use")
	set.Int(&s.cfg.Processes, "GUSTO_OPENSHELL_PROCESSES", openshell.DefaultProcesses, 1, "the most processes an agent may have running in a sandbox at once")
	set.Bool(&s.cfg.Remote, "GUSTO_OPENSHELL_REMOTE", "the gateway runs on another machine, so this one needs no hardware virtualization")
}

// gateway is the gateway's settings, with the hosts a task may reach besides its model.
func (s openShellSettings) gateway(allowedHosts string) openshell.Config {
	cfg := s.cfg
	cfg.Allowed = nil
	for _, entry := range strings.Split(allowedHosts, ",") {
		if entry = strings.TrimSpace(entry); entry != "" {
			cfg.Allowed = append(cfg.Allowed, entry)
		}
	}
	return cfg
}

// configuredKeyring is the keyring GUSTO_KEYRING names, for a command that reads records.
func configuredKeyring() (signing.Keyring, error) {
	var s signingSettings
	if err := loadSettings(s.declareKeyring); err != nil {
		return nil, err
	}
	return s.openKeyring()
}
