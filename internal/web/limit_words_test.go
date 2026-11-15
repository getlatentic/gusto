package web

import (
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/spend"
)

func TestAReplyStoppedByABudgetSaysWhyAndCallsItUsedUpOnlyWhenItIs(t *testing.T) {
	s := open(t).server
	record := tasks.Record{Asked: "grace@acme.com", Started: time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC).Unix()}
	used := s.limitWords(record, &spend.Refusal{Cap: "person", SpentUSD: 80, LimitUSD: 79.25})
	if !strings.Contains(used, "budget for November is used up, $80.00 of $79.25") {
		t.Errorf("a budget that is used up is said as %q", used)
	}
	unpriced := s.limitWords(record, &spend.Refusal{
		Cap: "person", SpentUSD: 0, LimitUSD: 79.25,
		Reason: "stub-model has no price in the spend policy, so the person cap cannot count it",
	})
	if strings.Contains(unpriced, "used up") || !strings.Contains(unpriced, "stub-model has no price in the spend policy") {
		t.Errorf("a call the budget cannot count is said as %q", unpriced)
	}
}
