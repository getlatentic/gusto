package desk

import "fmt"

// explain says why there is nowhere for an icon, from what the session has: a bus to reach the desktop, a program that shows the
// icons on it, and which desktop it is. It says nothing when there is somewhere.
func explain(hasBus, hasWatcher bool, desktop string) string {
	switch {
	case !hasBus:
		return "there is no desktop session to show an icon in (no D-Bus session): the helper runs without one"
	case !hasWatcher && isGNOME(desktop):
		return "GNOME does not show tray icons of its own: turn on the AppIndicator extension (\"AppIndicator and KStatusNotifierItem Support\") to see it; the helper runs without one"
	case !hasWatcher:
		return fmt.Sprintf("nothing on this desktop (%s) shows tray icons: the helper runs without one", orUnknown(desktop))
	}
	return ""
}

func isGNOME(desktop string) bool {
	for _, part := range splitDesktop(desktop) {
		if part == "GNOME" || part == "ubuntu:GNOME" {
			return true
		}
	}
	return false
}

func splitDesktop(desktop string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(desktop); i++ {
		if i == len(desktop) || desktop[i] == ':' {
			if i > start {
				parts = append(parts, desktop[start:i])
			}
			start = i + 1
		}
	}
	return parts
}

func orUnknown(desktop string) string {
	if desktop == "" {
		return "unknown"
	}
	return desktop
}
