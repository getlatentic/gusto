package usage

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/getlatentic/gusto/pkg/spend"
)

const (
	unknownPerson = "unknown"
	batchSize     = 500
)

// whoOf is who made a task's calls. A task the finder does not know is still counted, under the person
// "unknown" and with the task as its session, so that a cost is never lost for want of a name.
func whoOf(find WhoFunc, task string) Who {
	var who Who
	if find != nil {
		who, _ = find(task)
	}
	if who.Session == "" {
		who.Session = task
	}
	if who.Person == "" {
		who.Person = unknownPerson
	}
	return who
}

// rowOf is the one conversion from a line of a task's proxy record to a row, used by the live recorder and by a
// rebuild alike, so that both produce the same rows.
func rowOf(task string, line int, call spend.Call, who Who, provider ProviderFunc, fallback time.Time) Row {
	at := fallback
	if call.AtUnix != 0 {
		at = time.Unix(call.AtUnix, 0)
	}
	row := Row{
		Task: task, Line: line, At: at.UTC(),
		Session: who.Session, Person: who.Person, Team: who.Team, Project: who.Project, Engine: who.Engine, Agent: who.Agent,
		Credential: call.Credential, Model: call.Model,
		InputTokens: call.InputTokens, OutputTokens: call.OutputTokens, CachedTokens: call.CachedTokens, USD: call.USD,
		Status: call.Status, TookMS: call.Took, Translated: call.Translated,
	}
	if provider != nil {
		row.Provider = provider(call.Model, call.Credential)
	}
	if call.Refused != nil {
		row.Refused = call.Refused.Cap
	}
	return row
}

// parsed is what reading a task's proxy record from one line on found.
type parsed struct {
	Lines     int
	Rows      int
	Malformed int
}

// parseFrom reads a task's proxy record and hands the rows of its model calls, from line from on, to emit in
// batches. A line that is not a call is counted and passed over. A last line with no line feed may still be
// being written, so it is read only when it is whole.
func parseFrom(r io.Reader, task string, from int, who Who, provider ProviderFunc, fallback time.Time, emit func([]Row) error) (parsed, error) {
	var out parsed
	var batch []Row
	reader := bufio.NewReaderSize(r, 64*1024)
	for {
		text, err := reader.ReadBytes('\n')
		if len(text) > 0 {
			line := out.Lines
			out.Lines++
			if line >= from {
				if row, kind := rowFromText(text, task, line, who, provider, fallback, err == nil); kind == isRow {
					batch = append(batch, row)
					out.Rows++
				} else if kind == isMalformed {
					out.Malformed++
				}
			}
		}
		if len(batch) >= batchSize || (err != nil && len(batch) > 0) {
			if emitErr := emit(batch); emitErr != nil {
				return out, emitErr
			}
			batch = nil
		}
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
}

type lineKind int

const (
	isRow lineKind = iota
	isOther
	isMalformed
)

func rowFromText(text []byte, task string, line int, who Who, provider ProviderFunc, fallback time.Time, whole bool) (Row, lineKind) {
	if blank(text) {
		return Row{}, isOther
	}
	var call spend.Call
	if err := json.Unmarshal(text, &call); err != nil {
		if !whole {
			return Row{}, isOther
		}
		return Row{}, isMalformed
	}
	if !call.ToModel() {
		return Row{}, isOther
	}
	return rowOf(task, line, call, who, provider, fallback), isRow
}

func blank(text []byte) bool {
	for _, b := range text {
		if b != '\n' && b != '\r' && b != ' ' && b != '\t' {
			return false
		}
	}
	return true
}
