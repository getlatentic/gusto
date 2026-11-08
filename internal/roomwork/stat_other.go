//go:build !darwin && !linux

package roomwork

import "io/fs"

// stampOf is empty where the platform gives neither number, so two entries are told apart by their size, modified time and mode.
func stampOf(fs.FileInfo) stamp { return stamp{} }
