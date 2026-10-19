package helper

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Instance is one of the person's agents, as the server lists it.
type Instance struct {
	ID      string `json:"id"`
	Title   string `json:"title,omitempty"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
	// Folder is the key of the folder on a person's computer that the agent's folder is kept the same as, when there is one.
	Folder  string `json:"folder,omitempty"`
	Started int64  `json:"startedAt"`
}

// Thread is one conversation with an agent.
type Thread struct {
	ID       string `json:"id"`
	Instance string `json:"instance"`
	Title    string `json:"title"`
}

// Engine is an agent this server can start, and the models the person may use with it.
type Engine struct {
	Name   string   `json:"name"`
	Title  string   `json:"title"`
	Ready  bool     `json:"ready"`
	Why    string   `json:"why,omitempty"`
	Models []string `json:"models"`
}

// Said is what the server answers a send: the job that is the turn, and whether it was one made already by the same key.
type Said struct {
	Job     string `json:"job"`
	State   string `json:"state"`
	Already bool   `json:"already"`
}

// Step is one thing the agent did, in the words the pages use.
type Step struct {
	What   string `json:"what"`
	Detail string `json:"detail,omitempty"`
}

// Ended is a turn at its end: how it stood, what the agent said, and why it stopped when it did not finish.
type Ended struct {
	Job struct {
		ID        string `json:"id"`
		State     string `json:"state"`
		StoppedBy string `json:"stoppedBy,omitempty"`
	} `json:"job"`
	Steps   []Step   `json:"steps"`
	Reply   string   `json:"reply"`
	Changed []string `json:"changed"`
	Stopped string   `json:"stopped,omitempty"`
}

// Refused is the server saying no, with its reason.
type Refused struct {
	Status  int
	Message string
}

func (r *Refused) Error() string { return fmt.Sprintf("%s (%d)", r.Message, r.Status) }

// send makes a request with a JSON body and reads a JSON answer into out, or gives the server's reason for refusing.
func (c Client) send(ctx context.Context, method, path string, headers map[string]string, body, out any) error {
	var reader io.Reader
	if body != nil {
		text, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(text)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Server, "/")+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := c.http().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	text, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w: run `gusto-helper login` again", ErrSignedOut)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		var said struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(text, &said)
		if said.Error == "" {
			said.Error = strings.TrimSpace(http.StatusText(response.StatusCode))
		}
		return &Refused{Status: response.StatusCode, Message: said.Error}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(text, out)
}

// Instances lists the person's agents.
func (c Client) Instances(ctx context.Context) ([]Instance, error) {
	var out []Instance
	return out, c.send(ctx, http.MethodGet, "/api/instances", nil, nil, &out)
}

// Engines lists what the person may start.
func (c Client) Engines(ctx context.Context) ([]Engine, error) {
	var out []Engine
	return out, c.send(ctx, http.MethodGet, "/api/engines", nil, nil, &out)
}

// StartInstance starts an agent and gives it with its first thread.
func (c Client) StartInstance(ctx context.Context, harness, model string) (Instance, Thread, error) {
	var out struct {
		Instance Instance `json:"instance"`
		Thread   Thread   `json:"thread"`
	}
	err := c.send(ctx, http.MethodPost, "/api/instances", nil, map[string]string{"harness": harness, "model": model}, &out)
	return out.Instance, out.Thread, err
}

// NewThread opens a conversation on an agent.
func (c Client) NewThread(ctx context.Context, instance, title string) (Thread, error) {
	var out Thread
	return out, c.send(ctx, http.MethodPost, "/api/instances/"+url.PathEscape(instance)+"/threads", nil, map[string]string{"title": title}, &out)
}

// Say says something in a thread. The key makes a send that is tried again the same send.
func (c Client) Say(ctx context.Context, instance, thread, said, key string) (Said, error) {
	var out Said
	headers := map[string]string{"Idempotency-Key": key}
	return out, c.send(ctx, http.MethodPost, "/api/instances/"+url.PathEscape(instance)+"/threads/"+url.PathEscape(thread)+"/say", headers, map[string]string{"said": said}, &out)
}

// Stop stops a turn. A turn that is already over is not an error: it is as stopped as it will get.
func (c Client) StopJob(ctx context.Context, job string) error {
	err := c.send(ctx, http.MethodPost, "/api/jobs/"+url.PathEscape(job)+"/stop", nil, nil, nil)
	var refused *Refused
	if asRefused(err, &refused) && refused.Status == http.StatusConflict {
		return nil
	}
	return err
}

func asRefused(err error, target **Refused) bool {
	refused, ok := err.(*Refused)
	if ok {
		*target = refused
	}
	return ok
}

// Follower is told what a turn does as it goes.
type Follower struct {
	// Step is told each step the agent takes, once, in order.
	Step func(Step)
	// Reply is told what the agent has said so far, whenever that changes.
	Reply func(text string)
}

// FollowJob follows one turn until it is over, and gives it. A connection that drops before the end is an error, and following
// again from the start is safe: steps are told by their place, which the caller keeps.
func (c Client) FollowJob(ctx context.Context, job string, told Follower) (Ended, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.Server, "/")+"/api/jobs/"+url.PathEscape(job)+"/follow", nil)
	if err != nil {
		return Ended{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("Accept", "text/event-stream")
	response, err := (&http.Client{Transport: c.transport()}).Do(request)
	if err != nil {
		return Ended{}, err
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusUnauthorized:
		return Ended{}, fmt.Errorf("%w: run `gusto-helper login` again", ErrSignedOut)
	case response.StatusCode != http.StatusOK:
		return Ended{}, &Refused{Status: response.StatusCode, Message: "the turn could not be followed"}
	}
	lines := bufio.NewScanner(response.Body)
	lines.Buffer(make([]byte, 64<<10), 16<<20)
	var kind, data string
	for lines.Scan() {
		line := lines.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			kind = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case line == "":
			switch kind {
			case "step":
				var step Step
				if json.Unmarshal([]byte(data), &step) == nil && told.Step != nil {
					told.Step(step)
				}
			case "reply":
				var reply struct {
					Text string `json:"text"`
				}
				if json.Unmarshal([]byte(data), &reply) == nil && told.Reply != nil {
					told.Reply(reply.Text)
				}
			case "ended":
				var ended Ended
				return ended, json.Unmarshal([]byte(data), &ended)
			}
			kind, data = "", ""
		}
	}
	if err := lines.Err(); err != nil {
		return Ended{}, err
	}
	if ctx.Err() != nil {
		return Ended{}, ctx.Err()
	}
	return Ended{}, io.ErrUnexpectedEOF
}

// patient is how long a send is tried before it is given up on, with the same key each time.
const patient = 20 * time.Second

// RemoteFile is a file of an agent's folder as the listing gives it.
type RemoteFile struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	ModifiedNs int64  `json:"modifiedNs"`
}

// Files lists the regular files of an agent's folder, and says when the list was cut.
func (c Client) Files(ctx context.Context, instance string) (files []RemoteFile, cut bool, err error) {
	var out struct {
		Files []RemoteFile `json:"files"`
		Cut   bool         `json:"cut"`
	}
	err = c.send(ctx, http.MethodGet, "/api/instances/"+url.PathEscape(instance)+"/files", nil, nil, &out)
	return out.Files, out.Cut, err
}

// Hashes gives the hash of each file named. A file that is not there, or is not a plain file, is left out.
func (c Client) Hashes(ctx context.Context, instance string, paths []string) (map[string]string, error) {
	var out struct {
		Hashes map[string]string `json:"hashes"`
	}
	err := c.send(ctx, http.MethodPost, "/api/instances/"+url.PathEscape(instance)+"/hashes", nil, map[string][]string{"paths": paths}, &out)
	return out.Hashes, err
}

// Changed is the server refusing a write or a delete because the file is not what the writer saw. Hash is what it is.
type Changed struct{ Hash string }

func (c *Changed) Error() string { return "the file changed in the cloud since it was read" }

// Download writes a file of the agent's folder to w.
func (c Client) Download(ctx context.Context, instance, path string, w io.Writer) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.Server, "/")+"/api/instances/"+url.PathEscape(instance)+"/file?path="+url.QueryEscape(path), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	response, err := (&http.Client{Transport: c.transport()}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &Refused{Status: response.StatusCode, Message: "the file could not be read"}
	}
	_, err = io.Copy(w, response.Body)
	return err
}

// Upload puts a file in the agent's folder, whole. saw is the hash the file had when it was read, or "absent" when it was not
// there, and the server refuses with Changed when the file is something else now. It gives the hash the server computed.
func (c Client) Upload(ctx context.Context, instance, path string, body io.Reader, saw string, executable bool) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, strings.TrimRight(c.Server, "/")+"/api/instances/"+url.PathEscape(instance)+"/file?path="+url.QueryEscape(path), body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("X-If-Hash", saw)
	if executable {
		request.Header.Set("X-Mode", "755")
	}
	response, err := (&http.Client{Transport: c.transport()}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	var answer struct {
		Hash  string `json:"hash"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(text, &answer)
	switch response.StatusCode {
	case http.StatusOK:
		return answer.Hash, nil
	case http.StatusPreconditionFailed:
		return "", &Changed{Hash: answer.Hash}
	case http.StatusUnauthorized:
		return "", fmt.Errorf("%w: run `gusto-helper login` again", ErrSignedOut)
	}
	return "", &Refused{Status: response.StatusCode, Message: firstNonEmpty(answer.Error, "the file could not be put")}
}

// Remove deletes a file of the agent's folder, when it is what the writer saw.
func (c Client) Remove(ctx context.Context, instance, path, saw string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, strings.TrimRight(c.Server, "/")+"/api/instances/"+url.PathEscape(instance)+"/file?path="+url.QueryEscape(path), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	request.Header.Set("X-If-Hash", saw)
	response, err := c.http().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	var answer struct {
		Hash  string `json:"hash"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(text, &answer)
	switch response.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusPreconditionFailed:
		return &Changed{Hash: answer.Hash}
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: run `gusto-helper login` again", ErrSignedOut)
	}
	return &Refused{Status: response.StatusCode, Message: firstNonEmpty(answer.Error, "the file could not be removed")}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
