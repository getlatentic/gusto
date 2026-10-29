package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// A room's file card opens in the side panel and downloads under its own name, as it does in a chat.
func TestARoomsFileCardOpensInThePanelAndDownloadsUnderItsName(t *testing.T) {
	here, id, _ := aRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	if err := os.WriteFile(filepath.Join(here.server.instances.Workspace(id), "audience.md"), []byte("- Teams that run agents\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if added := here.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members", url.Values{"who": {bob}}); added.Code != http.StatusSeeOther {
		t.Fatalf("adding bob gave %d", added.Code)
	}
	for _, who := range []string{alice, bob} {
		openAndDownload(t, here, who, id)
	}
}

func openAndDownload(t *testing.T, here *site, who, id string) {
	t.Helper()
	panel := here.as(t, who).ask(t, "GET", "/instances/"+id+"/file?path=audience.md&panel=1", nil)
	if panel.Code != http.StatusOK {
		t.Errorf("%s: the panel answered %d: %.300s", who, panel.Code, panel.Body.String())
	}
	download := here.as(t, who).ask(t, "GET", "/instances/"+id+"/raw?path=audience.md&download=1", nil)
	if got := download.Header().Get("Content-Disposition"); download.Code != http.StatusOK || got != `attachment; filename=audience.md` {
		t.Errorf("%s: the download answered %d with %q", who, download.Code, got)
	}
}
