package cell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
)

// Remote is a host on another host, asked in the words of cloud.Cell.
type Remote struct {
	name, agent, private string
	secret               []byte
	client               *http.Client
	now                  func() time.Time
}

func newRemote(r Registration, secret []byte) *Remote {
	return &Remote{name: r.Name, agent: r.Agent, private: r.Private, secret: secret, client: &http.Client{Timeout: 10 * time.Second}, now: time.Now}
}

// Detached says the servers on this host outlive the gusto cloud that started them.
func (c *Remote) Detached() bool { return true }

func (c *Remote) Name() string   { return c.name }
func (c *Remote) Host() string   { return c.private }
func (c *Remote) Listen() string { return c.private }

// call sends a signed request to the agent and decodes what it says.
func (c *Remote) call(method, path string, body, result any, timeout time.Duration) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	request, err := http.NewRequest(method, "http://"+c.agent+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	sign(request, c.secret, c.now(), payload)
	client := *c.client
	if timeout > 0 {
		client.Timeout = timeout
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("the host %s did not answer: %w", c.name, err)
	}
	defer response.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(response.Body, maxBody))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the host %s said %d: %s", c.name, response.StatusCode, strings.TrimSpace(string(text)))
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(text, result)
}

func (c *Remote) Start(spec cloud.ServerSpec) error {
	return c.call("POST", "/v1/servers", startRequest{Slug: spec.Slug, Command: spec.Command, Env: spec.Env, Log: spec.Log}, nil, 0)
}

func (c *Remote) Stop(slug string, grace time.Duration) error {
	return c.call("POST", "/v1/servers/"+url.PathEscape(slug)+"/stop", stopRequest{GraceSeconds: int(grace / time.Second)}, nil, grace+15*time.Second)
}

// Alive is false when the host cannot be asked: a server that is not known to be running is not
// treated as running.
func (c *Remote) Alive(slug string) bool {
	var answer struct{ Alive bool }
	return c.call("GET", "/v1/servers/"+url.PathEscape(slug), nil, &answer, 3*time.Second) == nil && answer.Alive
}

func (c *Remote) Ports(from int, taken map[int]bool) (server, proxy int, err error) {
	var skip []string
	for port := range taken {
		skip = append(skip, strconv.Itoa(port))
	}
	sort.Strings(skip)
	var answer struct{ Server, Proxy int }
	err = c.call("GET", "/v1/ports?from="+strconv.Itoa(from)+"&taken="+strings.Join(skip, ","), nil, &answer, 0)
	return answer.Server, answer.Proxy, err
}

// Status is what the host says of itself.
func (c *Remote) Status() (Status, error) {
	var status Status
	return status, c.call("GET", "/v1/status", nil, &status, 3*time.Second)
}
