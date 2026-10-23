package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// Record is what a run writes while it happens. A container's disk does not
// survive a stop, so each line is on the bind-mounted task directory before the
// next one is read.
type Record struct {
	adapter    harness.Adapter
	raw        *os.File
	transcript *os.File
	calls      *os.File
	commands   *os.File
	errs       *os.File

	// Remembered is the agent's own name for the conversation, said in its
	// output. Handing it back is how the next turn continues rather than starts.
	Remembered string

	perCall struct {
		tokens int64
		cost   float64
	}
	latest struct{ tokens, cost float64 }
	seen   bool
	// message is the message the transcript's last text belonged to.
	message string
	// wrote is how many bytes of the transcript are written, so that what the agent ran can say where in what it said it ran it.
	wrote int64
	// priced is whether the harness ever named a cost. Tokens counted without
	// one is the ordinary case — most harnesses report numbers and no money —
	// and a cost of zero written there would read as a run that was free.
	priced bool
}

func Open(dir job.Dir, adapter harness.Adapter) (*Record, error) {
	record := &Record{adapter: adapter}
	for _, opening := range []struct {
		name string
		into **os.File
	}{
		{"harness.log", &record.raw},
		{"transcript.log", &record.transcript},
		{"calls.jsonl", &record.calls},
		{"commands.jsonl", &record.commands},
		{"stderr.log", &record.errs},
	} {
		file, err := os.Create(dir.OutFile(opening.name))
		if err != nil {
			return nil, err
		}
		*opening.into = file
	}
	return record, nil
}

// Out keeps one line of the agent's output: raw always, and the text and the
// numbers when the adapter says where they are.
func (r *Record) Out(line string) error {
	if _, err := r.raw.WriteString(line + "\n"); err != nil {
		return err
	}
	if r.adapter.Stream == harness.Plain {
		return r.say(line + "\n")
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		return nil
	}
	if named, ok := harness.Pick(decoded, r.adapter.SessionID); ok {
		if said, ok := named.(string); ok && said != "" {
			r.Remembered = said
		}
	}
	if text, ok := harness.Pick(decoded, r.adapter.Text); ok && r.adapter.Says(decoded) {
		if said, ok := text.(string); ok {
			if err := r.say(r.paragraph(decoded) + said); err != nil {
				return err
			}
		}
	}
	if err := r.did(decoded); err != nil {
		return err
	}
	return r.usage(decoded, line)
}

// say writes to the transcript and counts what it wrote.
func (r *Record) say(text string) error {
	written, err := r.transcript.WriteString(text)
	r.wrote += int64(written)
	return err
}

// paragraph is the break owed before text from a message other than the last.
func (r *Record) paragraph(decoded map[string]any) string {
	named, ok := harness.Pick(decoded, r.adapter.Message)
	if !ok {
		return ""
	}
	message := fmt.Sprint(named)
	started := r.message != "" && message != r.message
	r.message = message
	if started {
		return "\n\n"
	}
	return ""
}

// did keeps what the harness ran, apart from what it said. Somebody asking what
// an agent touched should not have to read a transcript to find out, and a tool
// call is the only place a harness says it. A line on which the adapter's tool
// name resolves is a tool call; nothing else distinguishes one.
func (r *Record) did(decoded map[string]any) error {
	name, isCall := harness.Pick(decoded, r.adapter.ToolName)
	if !isCall || !r.adapter.Ran(decoded) {
		return nil
	}
	ran := map[string]any{"tool": name, "at": r.wrote}
	for field, path := range map[string]string{
		"input":  r.adapter.ToolInput,
		"status": r.adapter.ToolStatus,
		"title":  r.adapter.ToolTitle,
	} {
		if value, found := harness.Pick(decoded, path); found {
			ran[field] = value
		}
	}
	line, err := json.Marshal(ran)
	if err != nil {
		return nil
	}
	_, err = r.commands.WriteString(string(line) + "\n")
	return err
}

func (r *Record) usage(decoded map[string]any, line string) error {
	if !r.adapter.Counts(decoded) {
		return nil
	}
	counted, told := r.tokens(decoded)
	cost, hasCost := harness.Number(decoded, r.adapter.CostUSD)
	if !told && !hasCost {
		return nil
	}
	r.seen = true
	r.priced = r.priced || hasCost
	if r.adapter.Totals == harness.Cumulative {
		r.latest.tokens, r.latest.cost = counted, cost
	} else {
		r.perCall.tokens += int64(counted)
		r.perCall.cost += cost
	}
	_, err := r.calls.WriteString(line + "\n")
	return err
}

// tokens is what one line counts: the harness's own total when it states one,
// and the sum of the parts otherwise.
func (r *Record) tokens(decoded map[string]any) (float64, bool) {
	if total, ok := harness.Number(decoded, r.adapter.TotalTokens); ok {
		return total, true
	}
	counted, told := 0.0, false
	for _, path := range []string{
		r.adapter.InputTokens,
		r.adapter.OutputTokens,
		r.adapter.CacheWriteTokens,
		r.adapter.CacheReadTokens,
	} {
		if number, ok := harness.Number(decoded, path); ok {
			counted += number
			told = true
		}
	}
	return counted, told
}

func (r *Record) Err(line string) error {
	_, err := r.errs.WriteString(line + "\n")
	return err
}

// Spent is what the agent reported. Nothing reported is nothing claimed, so
// the caller writes null rather than zero — and the two are asked separately,
// because an agent that counts its tokens and names no price is the ordinary
// case. Writing zero there would say a run was free, which is the one thing a
// record of spending must never say by accident.
func (r *Record) Spent() (*int64, *float64) {
	if !r.seen {
		return nil, nil
	}
	tokens, cost := r.perCall.tokens, r.perCall.cost
	if r.adapter.Totals == harness.Cumulative {
		tokens, cost = int64(r.latest.tokens), r.latest.cost
	}
	if !r.priced {
		return &tokens, nil
	}
	return &tokens, &cost
}

func (r *Record) Close() error {
	var failure error
	for _, file := range []*os.File{r.raw, r.transcript, r.calls, r.commands, r.errs} {
		if err := file.Close(); err != nil && failure == nil {
			failure = err
		}
	}
	return failure
}

// Tail is the last few lines of a file, for a person reading a summary.
func Tail(path string, lines int) string {
	text, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	all := strings.Split(strings.TrimRight(string(text), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return strings.Join(all, "\n")
}
