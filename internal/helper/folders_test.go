package helper

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type recordedFolders struct{ got map[string]string }

func (r *recordedFolders) Register(key, path string) error {
	if strings.Contains(path, "refused") {
		return errors.New("that is not a folder")
	}
	if r.got == nil {
		r.got = map[string]string{}
	}
	r.got[key] = path
	return nil
}
func (r *recordedFolders) Registered() map[string]string { return r.got }

// The desktop app registers a folder with the helper over its own address and token, and a refusal is said to it.
func TestTheAppRegistersAFolderWithTheHelper(t *testing.T) {
	o := newOrganization(t)
	vault := &memoryVault{}
	vault.Set(o.URL, "good")
	folders := &recordedFolders{}
	daemon, local, _ := running(t, o, vault, t.TempDir(), func(d *Daemon) { d.Folders = folders })
	_ = daemon
	post := func(body string) int {
		request, _ := http.NewRequest("POST", "http://"+local.Address+"/v1/folders", bytes.NewBufferString(body))
		request.Header.Set("Authorization", "Bearer "+local.Token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response.StatusCode
	}
	if code := post(`{"key":"0123456789abcdef","path":"/work/thesis"}`); code != http.StatusOK || folders.got["0123456789abcdef"] != "/work/thesis" {
		t.Errorf("a folder was answered %d and kept as %v", code, folders.got)
	}
	for _, bad := range []string{`{"key":"nope","path":"/work"}`, `{"key":"0123456789abcdef","path":"/refused"}`, `not json`} {
		if code := post(bad); code != http.StatusBadRequest {
			t.Errorf("%s was answered %d", bad, code)
		}
	}
}
