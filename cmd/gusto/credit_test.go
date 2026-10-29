package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/gusto/internal/quota"
)

// The credit is the one the organization's quota gives, read as the quota changes; a quota with none holds nothing to a credit,
// and one that cannot be read is a credit of nothing.
func TestTheCreditIsReadFromTheQuota(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.json")
	limits := quota.Open(path)
	credit := creditOf(limits)
	if usd, held := credit(); usd != 0 || !held {
		t.Errorf("an unreadable quota: %v %v", usd, held)
	}
	quota.Write(path, quota.Quota{Jobs: 3, CreditUSD: 5})
	if usd, held := creditOf(quota.Open(path))(); usd != 5 || !held {
		t.Errorf("%v %v", usd, held)
	}
	os.Remove(path)
	quota.Write(path, quota.Quota{Jobs: 3})
	if usd, held := creditOf(quota.Open(path))(); held {
		t.Errorf("a quota with no credit: %v %v", usd, held)
	}
}
