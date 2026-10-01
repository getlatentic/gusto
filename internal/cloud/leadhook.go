package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LeadHook sends a lead to the place it is followed up, which is a web address that takes it as JSON: when a visitor
// starts a demo, and again when the demo ends, with how. What is to be sent is written to Dir first and removed when
// the address has taken it, so a lead is not lost while the address is down, and is sent in the order it happened.
// URL and Secret are both kept out of the repository: the address is the way in, and the secret is in each request.
type LeadHook struct {
	URL, Secret string
	// Dir is where what is waiting to be sent is kept.
	Dir    string
	Client *http.Client
	mu     sync.Mutex
}

// Enabled says leads are sent.
func (h *LeadHook) Enabled() bool { return h != nil && h.URL != "" && h.Secret != "" }

// Complete says whether one of the two is set without the other.
func (h *LeadHook) Complete() error {
	if h == nil {
		return nil
	}
	if (h.URL == "") != (h.Secret == "") {
		return errors.New("the lead hook needs both its address and its secret")
	}
	if h.URL != "" {
		if parsed, err := url.Parse(h.URL); err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return errors.New("the lead hook's address is an https address")
		}
	}
	return nil
}

// Queue keeps an event to send. The files are named so that they sort in the order the events happened.
func (h *LeadHook) Queue(event string, lead Lead) error {
	if !h.Enabled() {
		return nil
	}
	if err := os.MkdirAll(h.Dir, 0o700); err != nil {
		return err
	}
	text, err := json.Marshal(map[string]any{"event": event, "lead": lead})
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%020d-%s-%s.json", time.Now().UnixNano(), event, lead.Slug)
	return os.WriteFile(filepath.Join(h.Dir, name), text, 0o600)
}

// Flush sends what is waiting, oldest first, and stops at the first that is not taken so that an event is never sent
// before one that came before it.
func (h *LeadHook) Flush(ctx context.Context) error {
	if !h.Enabled() {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	names, err := os.ReadDir(h.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var waiting []string
	for _, entry := range names {
		if strings.HasSuffix(entry.Name(), ".json") {
			waiting = append(waiting, entry.Name())
		}
	}
	sort.Strings(waiting)
	for _, name := range waiting {
		path := filepath.Join(h.Dir, name)
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := h.send(ctx, text); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

// send posts one event with the secret in its body, which is where the receiving side can read it. The answer must say
// that it was taken, since a page that answers 200 for a refused secret has not taken the lead.
func (h *LeadHook) send(ctx context.Context, event []byte) error {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(event, &body); err != nil {
		return err
	}
	secret, _ := json.Marshal(h.Secret)
	body["secret"] = secret
	text, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(text))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("the lead hook could not be reached")
	}
	defer response.Body.Close()
	answer, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	var taken struct {
		OK bool `json:"ok"`
	}
	if response.StatusCode != http.StatusOK || json.Unmarshal(answer, &taken) != nil || !taken.OK {
		return fmt.Errorf("the lead hook did not take the lead: %d", response.StatusCode)
	}
	return nil
}

// RunLeads sends the leads that are waiting every so often, until the context ends.
func (s *Server) RunLeads(ctx context.Context, every time.Duration) {
	if !s.Leads.Enabled() {
		return
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		if err := s.Leads.Flush(ctx); err != nil {
			slog.Warn("leads are waiting to be sent", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// tell queues a lead's event, and tries to send it at once so the sheet is current. A failure is kept for the next try.
func (s *Server) tell(event, slug string) {
	if !s.Leads.Enabled() {
		return
	}
	lead, found := s.Store.LeadOf(slug)
	if !found {
		return
	}
	if err := s.Leads.Queue(event, lead); err != nil {
		slog.Error("a lead was not kept to be sent", "org", slug, "event", event, "error", err)
		return
	}
	go func() {
		if err := s.Leads.Flush(context.Background()); err != nil {
			slog.Warn("a lead is waiting to be sent", "error", err)
		}
	}()
}
