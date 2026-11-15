package main

import (
	"fmt"
	"os"

	"github.com/getlatentic/gusto/internal/signing"
)

// scanCommand asks the runner image what it carries, which keeps the answer beside the image, so the
// servers that start after this find it there. A host does it before it takes any work, since the first
// question waits for the gateway to import the image.
func scanCommand() int { return scanFrom(os.Getenv) }

// scanFrom says what the boxes lookup sets offer.
func scanFrom(lookup func(string) string) int {
	cfg, _, err := loadServe(lookup)
	if err != nil {
		return fail(err)
	}
	boxes, err := openBackend(cfg, signing.Identity{}, nil, "")
	if err != nil {
		return fail(err)
	}
	for _, entry := range boxes.Offered() {
		if entry.Ready {
			fmt.Printf("%-10s offered %s\n", entry.Name, entry.Version)
		} else {
			fmt.Printf("%-10s not offered: %s\n", entry.Name, entry.Why)
		}
	}
	return 0
}
