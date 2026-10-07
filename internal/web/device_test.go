package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/changelog"
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
	approved := here.as(t, person).ask(t, "POST", "/device", url.Values{"code": {user}})
	if approved.Code != http.StatusOK || !strings.Contains(approved.Body.String(), "Approved") {
		t.Fatalf("the person's approval gave %d:\n%s", approved.Code, approved.Body)
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

func TestAPersonsTokenDoesNothingTheOrganisationsTokensDo(t *testing.T) {
	here := withTokens(t)
	secret := signInADevice(t, here, alice)
	for _, path := range []string{"/api/spend", "/api/changes", "/metrics"} {
		if got := here.call("GET", path, secret); got.Code != http.StatusForbidden && got.Code != http.StatusUnauthorized {
			t.Errorf("a person's token gave %d at %s", got.Code, path)
		}
	}
	if got := here.call("GET", "/api/me", here.secrets["everyone"]); got.Code == http.StatusOK {
		t.Errorf("an organisation's token passed for a person: %s", got.Body)
	}
}

func TestACodeIsGivenToOnePersonOnceAndTheSharedSignInIsNobody(t *testing.T) {
	here := withTokens(t)
	started := jsonOf(t, here.post("/api/device", `{}`, ""))
	device, user := started["device_code"].(string), started["user_code"].(string)

	if wrong := here.as(t, alice).ask(t, "POST", "/device", url.Values{"code": {"BCDF-GHJK"}}); !strings.Contains(wrong.Body.String(), "not waiting") {
		t.Errorf("a code nobody was given was not refused:\n%s", wrong.Body)
	}
	here.signIn(t)
	if shared := here.ask(t, "POST", "/device", url.Values{"code": {user}}); !strings.Contains(shared.Body.String(), "not one person") {
		t.Errorf("the shared sign-in approved a computer:\n%s", shared.Body)
	}
	if first := here.as(t, alice).ask(t, "POST", "/device", url.Values{"code": {strings.ToLower(user)}}); !strings.Contains(first.Body.String(), "Approved") {
		t.Fatalf("alice could not approve, in lower case:\n%s", first.Body)
	}
	if second := here.as(t, bob).ask(t, "POST", "/device", url.Values{"code": {user}}); !strings.Contains(second.Body.String(), "not waiting") {
		t.Errorf("bob approved what alice had:\n%s", second.Body)
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
	page := here.as(t, alice).ask(t, "GET", "/device", nil).Body.String()
	if !strings.Contains(page, "laptop-alice") || strings.Contains(page, "laptop-bob") {
		t.Fatalf("alice's page:\n%s", page)
	}
	id := regexpFirst(t, page, `/device/([0-9a-f]+)/revoke`)
	bobsID := regexpFirst(t, here.as(t, bob).ask(t, "GET", "/device", nil).Body.String(), `/device/([0-9a-f]+)/revoke`)
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
	if got := here.as(t, alice).ask(t, "GET", "/device", nil); got.Code != http.StatusNotFound {
		t.Errorf("the approval page answered %d", got.Code)
	}
}

func regexpFirst(t *testing.T, text, pattern string) string {
	t.Helper()
	found := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if len(found) < 2 {
		t.Fatalf("%s is not in:\n%s", pattern, text)
	}
	return found[1]
}
