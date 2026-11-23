package cloud

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// CreditUsed is what an organization has used of its credit, as its server last wrote it, and nothing before it has used any.
func (s *Store) CreditUsed(slug string) float64 {
	text, err := os.ReadFile(filepath.Join(s.DataDir(slug), "credit-used.json"))
	if err != nil {
		return 0
	}
	var used struct {
		USD float64 `json:"usd"`
	}
	if json.Unmarshal(text, &used) != nil {
		return 0
	}
	return used.USD
}
