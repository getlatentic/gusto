package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/web"
)

// submit sends a job directory to a server instead of running it here. It is the
// same directory `haas run` takes and the same spec: work moves off the laptop
// and outlives the shell without the job being written any differently.
func submit(args []string) int {
	dir, code := oneDir(args)
	if code != 0 {
		return code
	}
	server, err := sending(os.Getenv("HAAS_SERVER"))
	if err != nil {
		return fail(err)
	}
	secret := credential()
	if secret == "" {
		return fail(errors.New("set HAAS_TOKEN to an API token from the server's Tokens page, or HAAS_PASSWORD to its shared sign-in"))
	}
	spec, err := job.Read(dir)
	if err != nil {
		return fail(err)
	}
	// The adapter the job holds goes with it, because a server that has never
	// heard of this harness cannot run the job otherwise.
	adapter, err := carriedAdapter(dir, spec.Harness)
	if err != nil {
		return fail(err)
	}
	id, err := post(server, secret, web.Submission{Spec: spec, Adapter: adapter})
	if err != nil {
		return fail(err)
	}
	fmt.Printf("%s\n%s/jobs/%s\n", id, server, id)
	return 0
}

// credential is what to send the server: an API token when there is one, and
// otherwise the shared password a server with no tokens takes.
func credential() string {
	if token := os.Getenv("HAAS_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("HAAS_PASSWORD")
}

// sending is the server to send to. A scheme is asked for rather than assumed,
// because guessing http for a public name would put the shared password on the
// wire in the clear.
func sending(to string) (string, error) {
	to = strings.TrimSuffix(strings.TrimSpace(to), "/")
	if to == "" {
		return "", errors.New("set HAAS_SERVER to the server that runs the job, like https://haas.example.com")
	}
	if !strings.HasPrefix(to, "http://") && !strings.HasPrefix(to, "https://") {
		return "", fmt.Errorf("HAAS_SERVER needs a scheme: http://%s or https://%s", to, to)
	}
	return to, nil
}

// carriedAdapter reads the adapter the job holds for its harness. A job naming a
// built-in harness holds none, and every server has those already.
func carriedAdapter(dir job.Dir, name string) (*harness.Adapter, error) {
	text, err := os.ReadFile(filepath.Join(harness.Carried(dir), name+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var adapter harness.Adapter
	if err := json.Unmarshal(text, &adapter); err != nil {
		return nil, fmt.Errorf("the adapter this job carries for %s is not readable: %w", name, err)
	}
	return &adapter, nil
}

func post(server, secret string, sent web.Submission) (string, error) {
	body, err := json.Marshal(sent)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequest(http.MethodPost, server+"/api/jobs", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+secret)
	answer, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer answer.Body.Close()
	return taken(server, answer)
}

// taken reads the id out of what the server said, or the reason it refused. A
// reply that is not JSON at all is something other than haas at that address,
// which is worth saying plainly.
func taken(server string, answer *http.Response) (string, error) {
	var said web.Answer
	if err := json.NewDecoder(answer.Body).Decode(&said); err != nil {
		return "", fmt.Errorf("%s answered %s, and not in JSON", server, answer.Status)
	}
	if answer.StatusCode != http.StatusCreated {
		if said.Refused == "" {
			said.Refused = answer.Status
		}
		return "", fmt.Errorf("the server refused this job: %s", said.Refused)
	}
	return said.ID, nil
}
