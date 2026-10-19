package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const cloudflareAPI = "https://api.cloudflare.com/client/v4"

// zoneClient changes DNS records in one Cloudflare zone.
type zoneClient struct {
	api, token, zone string
	http             *http.Client
	zoneID           string
}

func newZoneClient(api, token, zone string) *zoneClient {
	if api == "" {
		api = cloudflareAPI
	}
	return &zoneClient{api: api, token: token, zone: zone, http: &http.Client{Timeout: 20 * time.Second}}
}

type record struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// call sends a request and reads the answer's result, or the reason Cloudflare gave.
func (z *zoneClient) call(method, path string, body, result any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequest(method, z.api+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+z.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := z.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	text, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	var answer struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(text, &answer); err != nil || !answer.Success {
		var why []string
		for _, each := range answer.Errors {
			why = append(why, fmt.Sprintf("%d %s", each.Code, each.Message))
		}
		return fmt.Errorf("cloudflare %s %s: HTTP %d %s", method, path, response.StatusCode, strings.Join(why, "; "))
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(answer.Result, result)
}

func (z *zoneClient) id() (string, error) {
	if z.zoneID != "" {
		return z.zoneID, nil
	}
	var zones []struct {
		ID string `json:"id"`
	}
	if err := z.call("GET", "/zones?name="+url.QueryEscape(z.zone), nil, &zones); err != nil {
		return "", err
	}
	if len(zones) != 1 {
		return "", fmt.Errorf("the token sees %d zones called %s, and needs to see one", len(zones), z.zone)
	}
	z.zoneID = zones[0].ID
	return z.zoneID, nil
}

func (z *zoneClient) find(name string) ([]record, error) {
	id, err := z.id()
	if err != nil {
		return nil, err
	}
	var found []record
	err = z.call("GET", "/zones/"+id+"/dns_records?type=A&name="+url.QueryEscape(name), nil, &found)
	return found, err
}

// point makes name an unproxied A record for ip with a short lifetime, changing the
// record that is there and leaving it alone when it is already right. Two records for
// one name would send half the visitors to a host that is gone.
func (z *zoneClient) point(name, ip string) error {
	found, err := z.find(name)
	if err != nil {
		return err
	}
	want := record{Type: "A", Name: name, Content: ip, TTL: 60}
	id, _ := z.id()
	for i, each := range found {
		if i > 0 {
			if err := z.call("DELETE", "/zones/"+id+"/dns_records/"+each.ID, nil, nil); err != nil {
				return err
			}
			continue
		}
		if each.Content == ip && each.TTL == want.TTL && !each.Proxied {
			continue
		}
		if err := z.call("PUT", "/zones/"+id+"/dns_records/"+each.ID, want, nil); err != nil {
			return err
		}
	}
	if len(found) == 0 {
		return z.call("POST", "/zones/"+id+"/dns_records", want, nil)
	}
	return nil
}

// remove deletes every A record of a name, and a name that has none is done.
func (z *zoneClient) remove(name string) error {
	found, err := z.find(name)
	if err != nil {
		return err
	}
	id, _ := z.id()
	for _, each := range found {
		if err := z.call("DELETE", "/zones/"+id+"/dns_records/"+each.ID, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

var errNoName = errors.New("-name is the name organisations are served under, such as gusto.example.com")
