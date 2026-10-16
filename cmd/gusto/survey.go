package main

import (
	"fmt"

	"github.com/getlatentic/gusto/internal/harness"
)

// surveyCommand asks the runner image what it carries, which keeps the answer beside the image, so the
// servers that start after this find it there. A host does it before it takes any work, since the first
// question waits for the gateway to import the image.
func surveyCommand([]string) int {
	offered := surveyed(harness.Names())
	for _, entry := range offered {
		if entry.Ready {
			fmt.Printf("%-10s offered\n", entry.Name)
		} else {
			fmt.Printf("%-10s not offered: %s\n", entry.Name, entry.Why)
		}
	}
	return 0
}
