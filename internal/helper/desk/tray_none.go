//go:build darwin && !cgo

package desk

import (
	"context"
	"errors"

	"github.com/getlatentic/gusto/internal/helper"
)

// ErrNoTray is this computer having nowhere to show an icon: a session with no desktop, or a build with no menu bar.
var ErrNoTray = errors.New("this computer has no place for a tray icon")

// Tray is the icon, and the menu behind it. This build has no menu bar: macOS draws it through the system's own libraries, and
// a build without a C compiler leaves them out.
type Tray struct {
	Hub    *helper.Hub
	Server string
	Status func() helper.Status
	Icon   []byte
	Shown  func()
}

// Run says there is no icon.
func (Tray) Run(context.Context) error {
	return errors.New("this build has no menu bar icon: it was built without a C compiler, which macOS needs for one")
}
