package repoaccess_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/repoaccess"
)

const appID = "424242"

// minted is one token the fake forge handed out, and what it was asked for.
type minted struct {
	Installation int64
	Repositories []string
	Permissions  map[string]string
}

// forge is a GitHub that holds private repositories: the API a GitHub App
// talks to, and git's own HTTP protocol, served by git itself, which answers
// only a request that carries a token this forge minted and has not revoked.
type forge struct {
	t       *testing.T
	server  *httptest.Server
	public  *rsa.PublicKey
	keyFile string
	guard   sync.Mutex
	// installed maps owner/name to the installation that reaches it.
	installed map[string]int64
	minted    []minted
	live      map[string]bool
	revoked   []string
	// authorised counts git requests that carried a live token.
	authorised int
	refused    int
	// root is where the forge keeps its bare repositories.
	root string
}

func newForge(t *testing.T) *forge {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "app.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(keyFile, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	f := &forge{t: t, public: &key.PublicKey, keyFile: keyFile, installed: map[string]int64{}, live: map[string]bool{}}
	root := t.TempDir()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/repos/{owner}/{name}/installation", f.installation)
	mux.HandleFunc("POST /api/app/installations/{id}/access_tokens", f.mint)
	mux.HandleFunc("DELETE /api/installation/token", f.revoke)
	backend := exec.Command("git", "--exec-path")
	execPath, err := backend.Output()
	if err != nil {
		t.Skip("git is not available")
	}
	mux.Handle("/", f.gated(&cgi.Handler{
		Path: filepath.Join(strings.TrimSpace(string(execPath)), "git-http-backend"),
		Root: "/",
		Env:  []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1"},
	}))
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	t.Setenv("GIT_ALLOW_PROTOCOL", "http:https:file")
	f.root = root
	return f
}

func (f *forge) forge() repoaccess.Forge {
	app, err := repoaccess.OpenApp(appID, f.keyFile, f.server.URL+"/api")
	if err != nil {
		f.t.Fatal(err)
	}
	return repoaccess.Forge{Web: f.server.URL, API: f.server.URL + "/api", App: app}
}

// env is the deployment's settings that name this forge.
func (f *forge) env(t *testing.T) {
	t.Helper()
	t.Setenv(repoaccess.AppIDEnv, appID)
	t.Setenv(repoaccess.AppKeyEnv, f.keyFile)
	t.Setenv(repoaccess.WebEnv, f.server.URL)
	t.Setenv(repoaccess.APIEnv, f.server.URL+"/api")
}

// private makes a repository with one file, installs the App on it when asked,
// and returns the address a job would name.
func (f *forge) private(owner, name string, installation int64) string {
	f.t.Helper()
	work := f.t.TempDir()
	run := func(dir string, args ...string) {
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if output, err := command.CombinedOutput(); err != nil {
			f.t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	run(work, "init", "--quiet", "-b", "main", ".")
	if err := os.WriteFile(filepath.Join(work, "secret.txt"), []byte("the private code\n"), 0o644); err != nil {
		f.t.Fatal(err)
	}
	run(work, "add", ".")
	run(work, "commit", "--quiet", "-m", "private")
	bare := filepath.Join(f.root, owner, name+".git")
	if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
		f.t.Fatal(err)
	}
	run(work, "clone", "--quiet", "--bare", ".", bare)
	if installation != 0 {
		f.installed[owner+"/"+name] = installation
	}
	return f.server.URL + "/" + owner + "/" + name + ".git"
}

func (f *forge) verifyJWT(r *http.Request) bool {
	bearer, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	parts := strings.Split(bearer, ".")
	if !found || len(parts) != 3 {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(f.public, crypto.SHA256, digest[:], signature) != nil {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return false
	}
	return claims.Iss == appID && claims.Exp-claims.Iat <= 600+60 && claims.Exp > time.Now().Unix() && claims.Iat <= time.Now().Unix()
}

func (f *forge) installation(w http.ResponseWriter, r *http.Request) {
	f.guard.Lock()
	defer f.guard.Unlock()
	if !f.verifyJWT(r) {
		http.Error(w, `{"message":"a JSON web token could not be decoded"}`, http.StatusUnauthorized)
		return
	}
	id, found := f.installed[r.PathValue("owner")+"/"+r.PathValue("name")]
	if !found {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	fmt.Fprintf(w, `{"id":%d}`, id)
}

func (f *forge) mint(w http.ResponseWriter, r *http.Request) {
	f.guard.Lock()
	defer f.guard.Unlock()
	if !f.verifyJWT(r) {
		http.Error(w, `{"message":"a JSON web token could not be decoded"}`, http.StatusUnauthorized)
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var asked struct {
		Repositories []string          `json:"repositories"`
		Permissions  map[string]string `json:"permissions"`
	}
	if json.NewDecoder(r.Body).Decode(&asked) != nil {
		http.Error(w, `{"message":"bad body"}`, http.StatusBadRequest)
		return
	}
	f.minted = append(f.minted, minted{Installation: id, Repositories: asked.Repositories, Permissions: asked.Permissions})
	token := fmt.Sprintf("ghs_test%016x", len(f.minted))
	f.live[token] = true
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"token":%q,"expires_at":%q}`, token, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
}

func (f *forge) revoke(w http.ResponseWriter, r *http.Request) {
	f.guard.Lock()
	defer f.guard.Unlock()
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !f.live[token] {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	delete(f.live, token)
	f.revoked = append(f.revoked, token)
	w.WriteHeader(http.StatusNoContent)
}

// gated answers git only with a live token, as a private repository does.
func (f *forge) gated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.guard.Lock()
		ok := false
		if header, found := strings.CutPrefix(r.Header.Get("Authorization"), "basic "); found {
			if raw, err := base64.StdEncoding.DecodeString(header); err == nil {
				user, token, _ := strings.Cut(string(raw), ":")
				ok = user == "x-access-token" && f.live[token]
			}
		}
		if ok {
			f.authorised++
		} else {
			f.refused++
		}
		f.guard.Unlock()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="forge"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
