package operator

import (
	"strings"
	"testing"
)

// The operator sees the notes visitors sent, and downloads them with what a visitor typed written as text, not as a formula.
func TestTheOperatorSeesTheNotesVisitorsSentAndDownloadsThem(t *testing.T) {
	a, store := app(t)
	if _, err := store.SaveContact(t.Context(), "maria@futureacmebank.com", "Future Acme Bank", "=HYPERLINK(\"http://evil.example\")"); err != nil {
		t.Fatal(err)
	}
	view := shown(t, a)
	if len(view.Contacts) != 1 || view.Contacts[0].Email != "maria@futureacmebank.com" || view.Contacts[0].Sent != "just now" {
		t.Errorf("the page lists %+v", view.Contacts)
	}
	got := ask(a, "GET", "/contacts.csv", "localhost:8090", nil, nil)
	body := got.Body.String()
	if !strings.HasPrefix(got.Header().Get("Content-Type"), "text/csv") || !strings.HasPrefix(body, "sent,email,company,note") {
		t.Fatalf("%v\n%s", got.Header(), body)
	}
	if !strings.Contains(body, "Future Acme Bank") || strings.Contains(body, ",=HYPERLINK") {
		t.Errorf("the spreadsheet is:\n%s", body)
	}
}
