package desk

import (
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

// With a real session bus, a desktop with nothing that shows icons is said to have nowhere for one, and the same desktop with a program that
// owns the watcher's name is not. It skips where there is no session bus, which a build machine seldom has.
func TestARealSessionBusSaysWhetherAnythingShowsTheIcon(t *testing.T) {
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skip("no session bus here")
	}
	defer bus.Close()
	t.Setenv("XDG_CURRENT_DESKTOP", "GNOME")
	if got := Missing(); !strings.Contains(got, "AppIndicator extension") {
		t.Fatalf("with nothing that shows icons: %q", got)
	}
	reply, err := bus.RequestName("org.kde.StatusNotifierWatcher", dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("the test could not stand in for a watcher: %v %v", reply, err)
	}
	if got := Missing(); got != "" {
		t.Errorf("with a watcher on the bus: %q", got)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	if got := Missing(); !strings.Contains(got, "no D-Bus session") {
		t.Errorf("with no session: %q", got)
	}
}
