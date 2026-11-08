package main

import (
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/sandbox"
	"github.com/getlatentic/gusto/internal/settings"
)

// taskSettings is how a task a command runs is held and who signs its record.
type taskSettings struct {
	containers containerSettings
	sign       signingSettings
}

func (s *taskSettings) declare(set *settings.Set) {
	s.containers.declare(set)
	s.sign.declareSigner(set)
}

// taskBoxes is the containers a command runs tasks in, with the signing key opened, or the exit code of why there are none.
func taskBoxes() (sandbox.Containers, int) {
	var s taskSettings
	if err := loadSettings(s.declare); err != nil {
		return sandbox.Containers{}, refused(err)
	}
	signs, err := s.sign.identity()
	if err != nil {
		return sandbox.Containers{}, fail(err)
	}
	return s.containers.signedBy(signs), 0
}

// insideSettings is what gusto inside is told of the place it runs in: in a task's box, by the box.
type insideSettings struct {
	sign        signingSettings
	proxySocket string
	agentUID    int
	keptHome    bool
}

func (s *insideSettings) declare(set *settings.Set) {
	s.sign.declareSigner(set)
	set.String(&s.proxySocket, "GUSTO_PROXY_SOCKET", "", "in a box with no network, the proxy's socket the agent's calls are relayed to")
	set.Int(&s.agentUID, "GUSTO_AGENT_UID", 0, 0, "the user the agent and the check run as when gusto runs as root in a box (default 0, which is gusto's own)")
	set.Bool(&s.keptHome, sandbox.KeptHomeEnv, "the agent's home is a person's own, kept between their tasks")
}

// insidePlace is where gusto inside runs, with the signing key opened, or the exit code of why it cannot run.
func insidePlace() (runner.Place, int) {
	var s insideSettings
	if err := loadSettings(s.declare); err != nil {
		return runner.Place{}, refused(err)
	}
	signs, err := s.sign.identity()
	if err != nil {
		return runner.Place{}, fail(err)
	}
	return runner.Place{Signing: signs, ProxySocket: s.proxySocket, AgentUID: s.agentUID, KeptHome: s.keptHome}, 0
}
