//go:build !linux

package desk

// Missing says why there is nowhere to show the icon, or nothing when there is. macOS and Windows always have a place for it.
func Missing() string { return "" }
