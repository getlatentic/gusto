// Package install holds the scripts that install gusto, so gusto cloud can serve them at its own address.
package install

import _ "embed"

// Script is install.sh, for macOS and Linux. Run with no argument, it installs the newest release, a pre-release included.
//
//go:embed install.sh
var Script []byte

// PowerShell is install.ps1, for Windows, which installs the newest release too unless GUSTO_TAG names one.
//
//go:embed install.ps1
var PowerShell []byte
