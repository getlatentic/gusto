package desk

import (
	_ "embed"
	"runtime"
)

//go:embed icon.png
var iconPNG []byte

//go:embed icon.ico
var iconICO []byte

// Icon is the picture in the tray, in the form the system takes: Windows wants an icon file and the others a PNG.
func Icon() []byte {
	if runtime.GOOS == "windows" {
		return iconICO
	}
	return iconPNG
}
