package spend

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"sync"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// A credit is what an organization may spend on the models its server provides, the operator's keys, over its whole life: a
// hosted organization's free start. It is not a month's cap and does not come back. What has been used is the most the ledger
// has ever counted on the operator's keys, kept in a file that only grows, so records deleted later do not give any back.

// CreditLimit is the credit there is now, read as it changes, and false when there is none. A limit that cannot be read is a
// credit of nothing, so a server that lost its limits spends nothing of the operator's.
type CreditLimit func() (float64, bool)

// creditUsed is the file that keeps what was used.
type creditUsed struct {
	USD float64 `json:"usd"`
}

// credit is the ledger's: the limit, and where what is used is kept. Used is base, the larger of what the file said and what the
// records held when the credit was taken up, and everything counted since, which is the ledger's count less start. The file is
// written outside the ledger's lock, and kept is what it was last written with.
type credit struct {
	limit CreditLimit
	path  string
	base  float64
	start float64
	kept  float64
	write sync.Mutex
}

// UseCredit holds the operator's keys to a credit, and keeps what is used at path. It is taken up after the ledger is rebuilt
// from the records. What the file already says was used counts, so a restart after records were deleted does not start the
// credit again, and the records that are still there are not counted twice.
func (l *Ledger) UseCredit(limit CreditLimit, path string) error {
	kept, err := readCreditUsed(path)
	if err != nil {
		return err
	}
	l.guard.Lock()
	l.credit = &credit{limit: limit, path: path, base: max(kept, l.operatorEver), start: l.operatorEver, kept: kept}
	used := l.creditUsedLocked()
	l.guard.Unlock()
	l.keepCreditUsed(used)
	return nil
}

func readCreditUsed(path string) (float64, error) {
	text, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var kept creditUsed
	if err := json.Unmarshal(text, &kept); err != nil {
		return 0, fmt.Errorf("what was used of the credit in %s cannot be read: %w", path, err)
	}
	return kept.USD, nil
}

// creditNow is the credit's limit as it was read for one decision, and false when there is none.
type creditNow struct {
	limit float64
	held  bool
}

// creditLimit reads the credit's limit, which may read a file, before the ledger's lock is taken.
func (l *Ledger) creditLimit() creditNow {
	l.guard.Lock()
	held := l.credit
	l.guard.Unlock()
	if held == nil {
		return creditNow{}
	}
	limit, found := held.limit()
	return creditNow{limit: limit, held: found}
}

// Credit is what has been used of the credit and what the credit is, and false when there is none.
func (l *Ledger) Credit() (used, limit float64, held bool) {
	now := l.creditLimit()
	if !now.held {
		return 0, 0, false
	}
	l.guard.Lock()
	defer l.guard.Unlock()
	return l.creditUsedLocked(), now.limit, true
}

// creditUsedLocked is what has been used of the credit.
func (l *Ledger) creditUsedLocked() float64 {
	return l.credit.base + l.operatorEver - l.credit.start
}

// creditSpent refuses a call on the operator's keys once the credit is used.
func (l *Ledger) creditSpent(now creditNow) *Refusal {
	if !now.held || l.credit == nil {
		return nil
	}
	limit, used := now.limit, l.creditUsedLocked()
	if used < limit {
		return nil
	}
	return &Refusal{
		Cap: "credit", LimitUSD: limit, SpentUSD: used,
		Reason: fmt.Sprintf("the free credit for the models this server provides is used, $%.2f of $%.2f: add your organization's own model key in Admin, Models, to carry on", used, limit),
	}
}

// keepCreditUsed writes what is used when it is more than the file says. Writes are in turn, so a smaller figure never
// replaces a larger one; one that fails is written by the next call.
func (l *Ledger) keepCreditUsed(used float64) {
	l.guard.Lock()
	held := l.credit
	l.guard.Unlock()
	if held == nil || held.path == "" {
		return
	}
	held.write.Lock()
	defer held.write.Unlock()
	l.guard.Lock()
	written := held.kept >= used
	l.guard.Unlock()
	if written {
		return
	}
	if err := atomicfile.Write(held.path, creditUsed{USD: used}, 0o600); err != nil {
		slog.Error("what was used of the credit was not kept", "path", held.path, "error", err)
		return
	}
	l.guard.Lock()
	held.kept = max(held.kept, used)
	l.guard.Unlock()
}
