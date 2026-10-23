package runner

import (
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/pkg/spend"
)

// stoppedByCap reads the proxy's record of the run that started at since for a
// cap that refused it.
// The proxy runs outside the box and writes into the job directory the box is
// given, so the runner learns why the model went quiet from the same file the
// manifest is about to hash, rather than from anything the harness said.
func stoppedByCap(dir job.Dir, since int64) (*evidence.CapStop, error) {
	refusal, err := spend.Stopped(dir.OutFile("proxy.jsonl"), since)
	if err != nil || refusal == nil {
		return nil, err
	}
	return &evidence.CapStop{
		Cap:      refusal.Cap,
		Team:     refusal.Team,
		LimitUSD: refusal.LimitUSD,
		SpentUSD: refusal.SpentUSD,
		Reason:   refusal.Reason,
	}, nil
}
