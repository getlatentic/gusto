package node

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// startedFile is where gusto cloud writes the address a node that has just started tells, in the directory every node sees.
const startedFile = "orchestrator.started"

// startedMost is the largest word a node sends when it starts: one registration.
const startedMost = 4 << 10

// TellStartedAt writes the address a node that has just started tells, so the organizations waiting for one are placed on it at
// once. A node reads its registration's directory for nothing else, and the shared file system cannot tell gusto cloud a new
// file is there.
func TellStartedAt(dir, address string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".started-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(address + "\n"); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), filepath.Join(dir, startedFile))
}

// sayStarted tells gusto cloud, at the address it wrote, that this node is there. A word that is not heard costs only time:
// gusto cloud finds the registration itself a few seconds later.
func (a *Agent) sayStarted(ctx context.Context, dir string, registration Registration) {
	address, err := os.ReadFile(filepath.Join(dir, startedFile))
	if err != nil {
		slog.Info("gusto cloud has not said where a started host tells it so", "error", err)
		return
	}
	body, err := json.Marshal(registration)
	if err != nil {
		return
	}
	asked, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(asked, http.MethodPost, strings.TrimSpace(string(address)), bytes.NewReader(body))
	if err != nil {
		slog.Error("the address a started host tells is not one", "error", err)
		return
	}
	request.Header.Set("Content-Type", "application/json")
	sign(request, a.Secret, a.clock(), body)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		slog.Error("gusto cloud was not told this host started", "error", err)
		return
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		slog.Error("gusto cloud did not take the word that this host started", "status", response.Status)
	}
}

// Started is what gusto cloud hears a node say when it starts: the node is held as registered, whatever the shared file system
// shows yet, and woken is told, so an organization waiting for room is placed on it at once.
func (p *Pool) Started(woken func()) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, startedMost)
		body, err := verify(r, p.Secret, p.clock())
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		var registration Registration
		if err := json.Unmarshal(body, &registration); err != nil || !ValidName(registration.Name) || registration.Agent == "" ||
			registration.Private == "" {
			http.Error(w, "that is not a host's registration", http.StatusBadRequest)
			return
		}
		p.hold(registration)
		woken()
		w.WriteHeader(http.StatusNoContent)
	})
}

// hold keeps a registration a node sent, until the shared file system shows it as new or newer.
func (p *Pool) hold(registration Registration) {
	p.guard.Lock()
	defer p.guard.Unlock()
	if p.started == nil {
		p.started = map[string]Registration{}
	}
	p.started[registration.Name] = registration
}

// registered is a node by its registration file, or by what it said when it started where that is newer.
func (p *Pool) registered(name string) (Registration, time.Duration, error) {
	now := p.clock()
	fromFile, age, err := Registered(p.Dir, name, now)
	p.guard.Lock()
	said, found := p.started[name]
	p.guard.Unlock()
	if found && (err != nil || said.Beat > fromFile.Beat) {
		return said, max(now.Sub(time.Unix(said.Beat, 0)), 0), nil
	}
	return fromFile, age, err
}

// fresh is every node heard from within maxAge, from its file or from what it said when it started, in name order.
func (p *Pool) fresh() ([]Registration, error) {
	now := p.clock()
	found, err := Fresh(p.Dir, now, p.maxAge())
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, r := range found {
		seen[r.Name] = true
	}
	p.guard.Lock()
	defer p.guard.Unlock()
	for name, said := range p.started {
		switch age := now.Sub(time.Unix(said.Beat, 0)); {
		case age > p.maxAge():
			// The file says as much by now, or the node is gone.
			delete(p.started, name)
		case !seen[name]:
			found = append(found, said)
		}
	}
	slices.SortFunc(found, func(a, b Registration) int { return strings.Compare(a.Name, b.Name) })
	return found, nil
}

// StartedAddress is where a node tells the gusto cloud at cloudURL that it has started.
func StartedAddress(cloudURL string) string {
	return strings.TrimSuffix(cloudURL, "/") + "/nodes/started"
}
