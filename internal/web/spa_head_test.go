package web

import (
	"strings"
	"testing"
)

func TestTheAppsPageKeepsItsHeaderAboveAPhonesKeyboard(t *testing.T) {
	if head := appHead(""); !strings.Contains(head, "/static/keyboard.js?v=") {
		t.Errorf("the app's page does not load the keyboard fix: %s", head)
	}
}

func TestTheAppsPageCarriesTheLeaseOnlyBehindGustoCloud(t *testing.T) {
	if head := appHead(""); strings.Contains(head, "lease.js") || strings.Contains(head, "guide.js") {
		t.Errorf("a server on its own loads the lease: %s", head)
	}
	head := appHead("https://cloud.test/lease?org=acme-com&x=1")
	if !strings.Contains(head, `data-extend="https://cloud.test/lease?org=acme-com&amp;x=1"`) || !strings.Contains(head, "/static/guide.js?v=") {
		t.Errorf("behind gusto cloud the app's page does not load the lease and its guide: %s", head)
	}
}
