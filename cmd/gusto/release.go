package main

import (
	"fmt"
	"runtime/debug"
)

// release is the release this binary is, set when a release is built:
// -ldflags "-X main.release=v0.1.0".
var release = ""

// releaseName is what a stamp and a version say this binary is: the release it
// was built as, and otherwise the commit it was built from.
func releaseName() string {
	if release != "" {
		return release
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) >= 7 {
				name := setting.Value[:7]
				for _, other := range info.Settings {
					if other.Key == "vcs.modified" && other.Value == "true" {
						name += "+modified"
					}
				}
				return name
			}
		}
	}
	return "dev"
}

// versionCommand is `gusto version`.
func versionCommand(args []string) int {
	if len(args) != 0 {
		usage()
		return 2
	}
	fmt.Println("gusto " + releaseName())
	return 0
}
