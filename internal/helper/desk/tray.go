//go:build !(darwin && !cgo)

package desk

import (
	"context"
	"errors"
	"os"
	"runtime"
	"time"

	"fyne.io/systray"

	"github.com/getlatentic/haas/internal/helper"
)

// ErrNoTray is this computer having nowhere to show an icon: a session with no desktop, or a build with no menu bar.
var ErrNoTray = errors.New("this computer has no place for a tray icon")

// Tray is the icon, and the menu behind it.
type Tray struct {
	Hub    *helper.Hub
	Server string
	// Status is how the helper stands, asked each time the menu is drawn.
	Status func() helper.Status
	Icon   []byte
}

// Run shows the icon and keeps its menu to the hub's turns until the person quits, which it says by returning nil, or the
// context ends. It runs on the program's main thread, which macOS needs.
func (t Tray) Run(ctx context.Context) error {
	if runtime.GOOS == "linux" && os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return ErrNoTray
	}
	go func() { <-ctx.Done(); systray.Quit() }()
	systray.Run(t.ready, func() {})
	return ctx.Err()
}

func (t Tray) ready() {
	systray.SetTemplateIcon(t.Icon, t.Icon)
	systray.SetTooltip("Latentic Agent Cloud")
	lines := make([]*systray.MenuItem, Slots+1)
	pages := make([]string, Slots+1)
	for i := range lines {
		lines[i] = systray.AddMenuItem("", "")
		lines[i].Hide()
		go func() {
			for range lines[i].ClickedCh {
				if pages[i] != "" {
					Open(pages[i])
				}
			}
		}()
	}
	systray.AddSeparator()
	web := systray.AddMenuItem("Open Latentic Agent Cloud", "")
	quit := systray.AddMenuItem("Quit", "")
	draw := func() {
		entries := Menu(t.Server, t.Status(), t.Hub.Turns())
		for i, line := range lines {
			if i >= len(entries) {
				line.Hide()
				continue
			}
			line.SetTitle(entries[i].Label)
			pages[i] = entries[i].Page
			if entries[i].Page == "" {
				line.Disable()
			} else {
				line.Enable()
			}
			line.Show()
		}
	}
	draw()
	events, stop := t.Hub.Listen()
	defer stop()
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-events:
		case <-tick.C:
		case <-web.ClickedCh:
			Open(t.Server)
		case <-quit.ClickedCh:
			systray.Quit()
			return
		}
		draw()
	}
}
