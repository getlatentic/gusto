// Package desk is the helper's part of a person's desktop: the icon in the menu bar or tray that lists their turns, and the
// notification that names one that ended.
package desk

import (
	"os/exec"
	"runtime"
)

// Open asks the operating system to open an address in the person's browser, and says nothing when it cannot.
func Open(address string) {
	_ = openCommand(runtime.GOOS, address).Start()
}

func openCommand(system, address string) *exec.Cmd {
	switch system {
	case "darwin":
		return exec.Command("open", address)
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	default:
		return exec.Command("xdg-open", address)
	}
}
