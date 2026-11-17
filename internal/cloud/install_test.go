package cloud

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/deploy/install"
)

// The front door serves the install scripts at its own address, which the docs name, so one short command installs the newest
// release whatever GitHub counts as its latest.
func TestTheFrontDoorServesTheInstallScripts(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, staffSet(), nil)
		status, body, header := f.browser().do("GET", "/install.sh", nil, nil)
		if status != 200 || body != string(install.Script) || !strings.HasPrefix(body, "#!/bin/sh") {
			t.Fatalf("%d: the install script is not served as it is: %.80q", status, body)
		}
		if got := header.Get("Content-Type"); !strings.HasPrefix(got, "text/x-shellscript") {
			t.Errorf("the script is served as %q", got)
		}
		if status, body, _ := f.browser().do("GET", "/install.ps1", nil, nil); status != 200 || body != string(install.PowerShell) {
			t.Errorf("%d: the Windows script is not served as it is", status)
		}
	})
}
