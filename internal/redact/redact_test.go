package redact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func all(t *testing.T) *Redactor {
	t.Helper()
	redactor, err := New(Names()...)
	if err != nil {
		t.Fatal(err)
	}
	return redactor
}

// request is a chat request holding the text given, as a harness sends it.
func request(text string) []byte {
	body, _ := json.Marshal(map[string]any{"model": "platform/qwen", "messages": []map[string]string{{"role": "user", "content": text}}})
	return body
}

func contentOf(t *testing.T, body []byte) string {
	t.Helper()
	var parsed struct {
		Messages []struct{ Content string } `json:"messages"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("what was left is not JSON: %v\n%s", err, body)
	}
	return parsed.Messages[0].Content
}

func TestEachKindIsMaskedAndCounted(t *testing.T) {
	cases := []struct{ kind, text string }{
		{"email", "write to ada.lovelace+work@analytical-engine.example.com about it"},
		{"card", "my card is 4111 1111 1111 1111, expires soon"},
		{"card", "pay with 5555-5555-5555-4444 please"},
		{"key", "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE"},
		{"key", "token ghp_" + strings.Repeat("a1B2", 9)},
		{"key", "ANTHROPIC_API_KEY=sk-ant-api03-" + strings.Repeat("Ab1_", 10)},
		{"key", "OPENAI_API_KEY=sk-proj-" + strings.Repeat("Zz9", 20)},
		{"key", "slack xoxb-123456789012-abcdefghij"},
		{"key", "stripe sk_live_" + strings.Repeat("a1", 14)},
		{"key", "jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r"},
		{"key", "curl -H 'Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123456789'"},
		{"key", "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA7\nabcdef\n-----END RSA PRIVATE KEY-----"},
	}
	for _, c := range cases {
		masked, counts := all(t).JSON(request(c.text))
		got := contentOf(t, masked)
		if counts[c.kind] != 1 || !strings.Contains(got, "[redacted:"+c.kind+"]") {
			t.Errorf("%s: %q became %q (%v)", c.kind, c.text, got, counts)
		}
		for _, secret := range []string{"ada.lovelace", "4111", "5555-5555", "AKIAIOSFODNN7EXAMPLE", "ghp_", "sk-ant", "sk-proj", "xoxb", "sk_live", "eyJhbGci", "abcdefghijklmnopqrstuvwxyz0123456789", "MIIEow"} {
			if strings.Contains(string(masked), secret) && strings.Contains(c.text, secret) {
				t.Errorf("%s: %q is still in %s", c.kind, secret, masked)
			}
		}
	}
}

func TestASecretAtTheStartOfALineIsFoundAfterTheEscapeForANewLine(t *testing.T) {
	text := "env file:\nAWS_ACCESS_KEY_ID=x\nAKIAIOSFODNN7EXAMPLE\n4111111111111111\nada@example.com\n"
	masked, counts := all(t).JSON(request(text))
	if counts["key"] != 1 || counts["card"] != 1 || counts["email"] != 1 {
		t.Errorf("counts = %v in %s", counts, masked)
	}
	if got := contentOf(t, masked); got != "env file:\nAWS_ACCESS_KEY_ID=x\n[redacted:key]\n[redacted:card]\n[redacted:email]\n" {
		t.Errorf("content = %q", got)
	}
}

func TestWhatIsNotASecretIsLeftAlone(t *testing.T) {
	text := "order 4111 1111 1111 1112 failed, id 12345678901234567, at 2026-10-08T09:00:00Z, see src/user@v2, version 1.2.3, sk-short, AKIA123"
	masked, counts := all(t).JSON(request(text))
	if len(counts) != 0 {
		t.Errorf("counts = %v, and nothing there is a secret", counts)
	}
	if contentOf(t, masked) != text {
		t.Errorf("the text changed: %q", contentOf(t, masked))
	}
	if string(masked) != string(request(text)) {
		t.Error("a message with nothing to mask was not left as the same bytes")
	}
}

func TestOnlyTheStringsThatHoldASecretAreRewrittenAndTheRestIsTheBytesItCameAs(t *testing.T) {
	body := `{"model":"m","temperature":0.70,"stream":true,"messages":[{"role":"user","content":"hi <b>&</b> é"},{"role":"user","content":"mail ada@example.com é <x>"}],"n":1e3}`
	masked, counts := all(t).JSON([]byte(body))
	want := `{"model":"m","temperature":0.70,"stream":true,"messages":[{"role":"user","content":"hi <b>&</b> é"},{"role":"user","content":"mail [redacted:email] é <x>"}],"n":1e3}`
	if string(masked) != want || counts["email"] != 1 {
		t.Errorf("\ngot  %s\nwant %s\n%v", masked, want, counts)
	}
}

func TestAMessageStaysValidJSONWhateverWasMasked(t *testing.T) {
	for _, text := range []string{
		"quote \" and backslash \\ then ada@example.com \"",
		"\\nada@example.com",
		"ada@example.com\\",
		"unicode   4111111111111111 \U0001F600",
		strings.Repeat("a", 5000) + " AKIAIOSFODNN7EXAMPLE " + strings.Repeat("b", 5000),
	} {
		masked, counts := all(t).JSON(request(text))
		if !json.Valid(masked) || counts.Total() != 1 {
			t.Errorf("%q: %v valid=%v", text[:min(len(text), 30)], counts, json.Valid(masked))
		}
	}
}

func TestACardWrittenAsANumberInJSONIsMaskedToo(t *testing.T) {
	body := `{"tool":"charge","arguments":{"card":4111111111111111,"amount":12,"id":1234567890123456789}}`
	masked, counts := all(t).JSON([]byte(body))
	if counts["card"] != 1 || strings.Contains(string(masked), "4111111111111111") || !strings.Contains(string(masked), `"amount":12`) {
		t.Errorf("%s %v", masked, counts)
	}
	if !json.Valid(masked) || !strings.Contains(string(masked), `"card":"[redacted:card]"`) {
		t.Errorf("a masked number is not a value any more: %s", masked)
	}
}

func TestAKindThatWasNotChosenIsNotMasked(t *testing.T) {
	only, err := New("card")
	if err != nil {
		t.Fatal(err)
	}
	masked, counts := only.JSON(request("ada@example.com and 4111111111111111"))
	if counts["email"] != 0 || counts["card"] != 1 || !strings.Contains(contentOf(t, masked), "ada@example.com") {
		t.Errorf("%v %q", counts, contentOf(t, masked))
	}
	if _, err := New("emial"); err == nil {
		t.Error("a misspelt kind was taken")
	}
	var none *Redactor
	if out, counts := none.JSON(request("ada@example.com")); counts != nil || !strings.Contains(string(out), "ada@example.com") {
		t.Error("no redactor masked something")
	}
	if nothing, _ := New(); nothing.On() {
		t.Error("a redactor of no kinds is on")
	}
}

func TestAStreamsEventsAreMaskedOneAtATimeAndKeepTheirShape(t *testing.T) {
	event := []byte("event: content_block_delta\ndata: {\"type\":\"delta\",\"text\":\"mail ada@example.com now\"}\n")
	masked, counts := all(t).Event(event)
	want := "event: content_block_delta\ndata: {\"type\":\"delta\",\"text\":\"mail [redacted:email] now\"}\n"
	if string(masked) != want || counts["email"] != 1 {
		t.Errorf("%q %v", masked, counts)
	}
	if out, counts := all(t).Event([]byte("data: [DONE]\n")); string(out) != "data: [DONE]\n" || counts != nil {
		t.Errorf("%q %v", out, counts)
	}
	if out, _ := all(t).Event([]byte("data:ada@example.com\n")); string(out) != "data:[redacted:email]\n" {
		t.Errorf("a data line with no space: %q", out)
	}
}

func TestTheConfigNamesKindsAndRefusesWhatItDoesNotKnow(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "redaction.json")
	os.WriteFile(good, []byte(`{"patterns":["email","key"]}`), 0o644)
	redactor, err := ReadConfig(good)
	if err != nil || strings.Join(redactor.Kinds(), ",") != "email,key" {
		t.Fatalf("%v %v", redactor.Kinds(), err)
	}
	if redactor, err := ReadConfig(filepath.Join(dir, "none.json")); err != nil || redactor.On() {
		t.Errorf("a missing file masks nothing: %v %v", redactor.On(), err)
	}
	if redactor, err := ReadConfig(""); err != nil || redactor.On() {
		t.Errorf("no file masks nothing: %v %v", redactor.On(), err)
	}
	for name, text := range map[string]string{"a misspelt kind": `{"patterns":["emial"]}`, "a misspelt field": `{"pattern":["email"]}`, "not JSON": `{`} {
		bad := filepath.Join(dir, "bad.json")
		os.WriteFile(bad, []byte(text), 0o644)
		if _, err := ReadConfig(bad); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
}

func BenchmarkAMegabyteOfConversation(b *testing.B) {
	redactor, _ := New(Names()...)
	body := request(strings.Repeat("Some ordinary text about a repository, a function and a test that fails on line 42. ", 12000))
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		redactor.JSON(body)
	}
}

func BenchmarkAMegabyteThatIsMostlySecrets(b *testing.B) {
	redactor, _ := New(Names()...)
	body := request(strings.Repeat("contact ada@example.com card 4111 1111 1111 1111 key AKIAIOSFODNN7EXAMPLE then more. ", 12000))
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		redactor.JSON(body)
	}
}
