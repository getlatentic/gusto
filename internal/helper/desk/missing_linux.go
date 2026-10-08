package desk

import (
	"os"

	"github.com/godbus/dbus/v5"
)

// Missing says why a Linux session has nowhere to show the icon, or nothing when it has. The icon is shown by a program on the
// desktop that owns the name org.kde.StatusNotifierWatcher on the session bus: KDE and most others have one, and GNOME has one only
// with an extension.
func Missing() string {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return explain(false, false, "")
	}
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		return explain(false, false, "")
	}
	defer bus.Close()
	owned := false
	_ = bus.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&owned)
	return explain(true, owned, os.Getenv("XDG_CURRENT_DESKTOP"))
}
