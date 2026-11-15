package web

import (
	"encoding/json"
	"github.com/coreos/go-oidc/v3/oidc"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
)

// post sends a JSON body the way a person's computer does, with no cookie.
func (s *site) post(path, body, bearer string) *httptest.ResponseRecorder {
	request := httptest.NewRequest("POST", path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

func jsonOf(t *testing.T, got *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &out); err != nil {
		t.Fatalf("%d %q is not JSON: %v", got.Code, got.Body.String(), err)
	}
	return out
}

// signInADevice runs the whole flow for a person and gives the token their computer was handed.
func signInADevice(t *testing.T, here *tokened, person string) string {
	t.Helper()
	started := jsonOf(t, here.post("/api/device", `{"label":"Laptop"}`, ""))
	device, user := started["device_code"].(string), started["user_code"].(string)
	if device == "" || user == "" {
		t.Fatalf("no codes were given: %+v", started)
	}
	if approved := sendCode(t, here.as(t, person), user); approved.Approved == "" {
		t.Fatalf("the person's approval was not said: %+v", approved)
	}
	got := here.post("/api/device/token", `{"device_code":"`+device+`"}`, "")
	if got.Code != http.StatusOK {
		t.Fatalf("the token was not handed over: %d %s", got.Code, got.Body)
	}
	return jsonOf(t, got)["access_token"].(string)
}

func TestAComputerSignsInAsAPersonByTheDeviceFlow(t *testing.T) {
	here := withTokens(t)
	if wait := here.post("/api/device/token", `{"device_code":"nobody-asked"}`, ""); wait.Code != http.StatusBadRequest || jsonOf(t, wait)["error"] != "expired_token" {
		t.Errorf("a code nobody was given answered %d %s", wait.Code, wait.Body)
	}
	started := jsonOf(t, here.post("/api/device", `{"label":"Laptop"}`, ""))
	if pending := here.post("/api/device/token", `{"device_code":"`+started["device_code"].(string)+`"}`, ""); pending.Code != http.StatusBadRequest || jsonOf(t, pending)["error"] != "authorization_pending" {
		t.Errorf("a code nobody approved answered %d %s", pending.Code, pending.Body)
	}

	secret := signInADevice(t, here, alice)
	me := here.call("GET", "/api/me", secret)
	if me.Code != http.StatusOK || jsonOf(t, me)["person"] != alice || jsonOf(t, me)["role"] != "runner" {
		t.Fatalf("the token did not say who it is: %d %s", me.Code, me.Body)
	}
	events := written(t, here.log)
	if made := ofKind(events, changelog.TokenCreated); len(made) == 0 || made[len(made)-1].Who != alice {
		t.Errorf("the token's making was not recorded against alice: %+v", events)
	}
}

func TestAPersonsTokenDoesNothingTheOrganizationsTokensDo(t *testing.T) {
	here := withTokens(t)
	secret := signInADevice(t, here, alice)
	for _, path := range []string{"/api/spend", "/api/changes", "/metrics"} {
		if got := here.call("GET", path, secret); got.Code != http.StatusForbidden && got.Code != http.StatusUnauthorized {
			t.Errorf("a person's token gave %d at %s", got.Code, path)
		}
	}
	if got := here.call("GET", "/api/me", here.secrets["everyone"]); got.Code == http.StatusOK {
		t.Errorf("an organization's token passed for a person: %s", got.Body)
	}
}

func TestACodeIsGivenToOnePersonOnceAndTheSharedSignInIsNobody(t *testing.T) {
	here := withTokens(t)
	here.server.identity = &Identity{provider: &oidc.Provider{}}
	started := jsonOf(t, here.post("/api/device", `{}`, ""))
	device, user := started["device_code"].(string), started["user_code"].(string)

	if wrong := sendCode(t, here.as(t, alice), "BCDF-GHJK"); !strings.Contains(wrong.Refused, "not waiting") {
		t.Errorf("a code nobody was given was not refused: %+v", wrong)
	}
	here.signIn(t)
	if shared := sendCode(t, here.site, user); !strings.Contains(shared.Refused, "not one person") || shared.Approved != "" {
		t.Errorf("the shared sign-in approved a computer: %+v", shared)
	}
	if first := sendCode(t, here.as(t, alice), strings.ToLower(user)); first.Approved != "computer" {
		t.Fatalf("alice could not approve, in lower case: %+v", first)
	}
	if second := sendCode(t, here.as(t, bob), user); !strings.Contains(second.Refused, "not waiting") {
		t.Errorf("bob approved what alice had: %+v", second)
	}
	if got := here.post("/api/device/token", `{"device_code":"`+device+`"}`, ""); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	if again := here.post("/api/device/token", `{"device_code":"`+device+`"}`, ""); again.Code != http.StatusBadRequest {
		t.Errorf("a code gave a token twice: %d %s", again.Code, again.Body)
	}
}

func TestAPersonSeesAndEndsOnlyTheirOwnComputers(t *testing.T) {
	here := withTokens(t)
	hers, his := signInADevice(t, here, alice), signInADevice(t, here, bob)
	page := devices(t, here.as(t, alice), "/device")
	if len(page.Tokens) != 1 || !strings.HasPrefix(page.Tokens[0].Name, "laptop-alice") || !page.Tokens[0].Live || page.Tokens[0].Made == "" {
		t.Fatalf("alice's page: %+v", page)
	}
	id := page.Tokens[0].ID
	bobsID := devices(t, here.as(t, bob), "/device").Tokens[0].ID
	if got := here.as(t, alice).ask(t, "POST", "/device/"+bobsID+"/revoke", nil); got.Code != http.StatusNotFound {
		t.Errorf("alice ended bob's computer: %d", got.Code)
	}
	if got := here.call("GET", "/api/me", his); got.Code != http.StatusOK {
		t.Errorf("bob's token stopped working: %d", got.Code)
	}
	if got := here.as(t, alice).ask(t, "POST", "/device/"+id+"/revoke", nil); got.Code != http.StatusSeeOther {
		t.Fatalf("alice could not end her own: %d", got.Code)
	}
	if got := here.call("GET", "/api/me", hers); got.Code != http.StatusUnauthorized {
		t.Errorf("an ended token still worked: %d", got.Code)
	}
	if ended := devices(t, here.as(t, alice), "/device").Tokens[0]; ended.Live || !strings.Contains(ended.Ended, "by "+alice) {
		t.Errorf("an ended computer is not said to be ended, and by whom: %+v", ended)
	}
	if revoked := ofKind(written(t, here.log), changelog.TokenRevoked); len(revoked) != 1 || revoked[0].Who != alice {
		t.Errorf("ending a token was not recorded against alice: %+v", revoked)
	}
}

func TestOneSourceCannotFillTheServerWithCodes(t *testing.T) {
	here := withTokens(t)
	var refused bool
	for i := 0; i < devicesPerClient+2; i++ {
		if here.post("/api/device", `{}`, "").Code == http.StatusTooManyRequests {
			refused = true
		}
	}
	if !refused {
		t.Errorf("one source asked for %d codes and none was refused", devicesPerClient+2)
	}
}

func TestAServerWithNoTokensHasNoDeviceFlow(t *testing.T) {
	here := open(t)
	if got := here.post("/api/device", `{}`, ""); got.Code != http.StatusNotFound {
		t.Errorf("a server with no token file answered %d", got.Code)
	}
	for _, path := range []string{"/device", "/app/api/device"} {
		if got := here.as(t, alice).ask(t, "GET", path, nil); got.Code != http.StatusNotFound {
			t.Errorf("%s answered %d", path, got.Code)
		}
	}
}

// devicePageJSON is the device page's data.
type devicePageJSON struct {
	Code, Refused, Approved string
	Shared, Owner           bool
	Tokens                  []struct {
		ID, Name, Made, LastUsed, Ended string
		Live                            bool
	}
}

// devices is the device page's data as who reads it, at the address given.
func devices(t *testing.T, who *site, path string) devicePageJSON {
	t.Helper()
	var page devicePageJSON
	who.data(t, path, &page)
	return page
}

// sendCode sends a code from the device page as who, and gives the page the server sent them back to, which says what became of it.
func sendCode(t *testing.T, who *site, code string) devicePageJSON {
	t.Helper()
	got := who.ask(t, "POST", "/device", url.Values{"code": {code}})
	if got.Code != http.StatusSeeOther {
		t.Fatalf("sending a code answered %d: %s", got.Code, got.Body)
	}
	return devices(t, who, got.Header().Get("Location"))
}

// With no identity provider the shared password is the one owner of the server, which is a developer's own: it approves a computer,
// and that computer sees the work the owner made in the pages as well as its own.
func TestWithNoIdentityProviderTheSharedPasswordIsTheOneOwnerAndItsComputerSeesTheirWork(t *testing.T) {
	here := withTokens(t)
	here.signIn(t)
	started := jsonOf(t, here.post("/api/device", `{"label":"Laptop"}`, ""))
	device, user := started["device_code"].(string), started["user_code"].(string)
	if page := devices(t, here.site, "/device"); !page.Owner || page.Shared {
		t.Errorf("the page does not say who the owner is: %+v", page)
	}
	if approved := sendCode(t, here.site, user); approved.Approved == "" {
		t.Fatalf("the owner could not approve a computer: %+v", approved)
	}
	got := here.post("/api/device/token", `{"device_code":"`+device+`"}`, "")
	if got.Code != http.StatusOK {
		t.Fatalf("the token was not handed over: %d %s", got.Code, got.Body)
	}
	token := jsonOf(t, got)["access_token"].(string)

	if made := here.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}}); made.Code != http.StatusSeeOther {
		t.Fatalf("the owner's page did not start an instance: %d", made.Code)
	}
	listed := here.call("GET", "/api/instances", token)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "fixture") {
		t.Errorf("the owner's computer does not see the instance the owner started in the pages: %d %s", listed.Code, listed.Body)
	}
	if page := devices(t, here.site, "/device"); len(page.Tokens) != 1 || !strings.HasPrefix(page.Tokens[0].Name, "laptop") {
		t.Errorf("the owner does not see their computer listed: %+v", page)
	}
}
