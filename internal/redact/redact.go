// Package redact masks personal data and secrets in what passes through the proxy and the gateway, so that using an agent does not
// send an email address, a card number or a key to a model provider, or hand one back into a conversation.
//
// A JSON message is read string by string. A string is unescaped before its patterns are looked for, so that a key at the start of
// a line, after the escape for a new line, is found; and it is written back escaped only when something in it was masked, so that
// the rest of the message is the bytes it came as. What was masked is counted by name, and the text is not kept.
//
// It finds what a pattern describes in one piece. A value that a model writes across two events of a stream is not found by the
// first and is not whole in either.
package redact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Counts is how many of each kind were masked.
type Counts map[string]int

// Add folds another count in.
func (c Counts) Add(other Counts) {
	for name, n := range other {
		c[name] += n
	}
}

// Total is how many were masked.
func (c Counts) Total() int {
	total := 0
	for _, n := range c {
		total += n
	}
	return total
}

type rule struct {
	name string
	find *regexp.Regexp
	// valid, when set, decides whether a match is one. A number that looks like a card and fails its check digit is not.
	valid func(match string) bool
	// hints are strings one of which a message must hold for the pattern to be worth looking for; scan, when set, says it instead.
	// A message of a few megabytes is read once for each, which costs far less than a pattern run over all of it.
	hints [][]byte
	scan  func(message []byte) bool
}

func (r rule) worthIn(message []byte) bool {
	if r.scan != nil {
		return r.scan(message)
	}
	for _, hint := range r.hints {
		if bytes.Contains(message, hint) {
			return true
		}
	}
	return false
}

func hint(words ...string) [][]byte {
	out := make([][]byte, len(words))
	for i, word := range words {
		out[i] = []byte(word)
	}
	return out
}

// kinds are the kinds this knows, in the order they are looked for. Keys come first so that the digits in one are not read as a
// card. A kind may be several patterns.
var kinds = []rule{
	{name: "key", find: regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`), hints: hint("-----BEGIN ")},
	{name: "key", find: regexp.MustCompile(`\b(?:AKIA|ASIA|AGPA|AIDA|AROA|ANPA)[0-9A-Z]{16}\b`), hints: hint("AKIA", "ASIA", "AGPA", "AIDA", "AROA", "ANPA")},
	{name: "key", find: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`), hints: hint("ghp_", "gho_", "ghu_", "ghs_", "ghr_")},
	{name: "key", find: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{50,}\b`), hints: hint("github_pat_")},
	{name: "key", find: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`), hints: hint("sk-ant-")},
	{name: "key", find: regexp.MustCompile(`\bsk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{32,}`), hints: hint("sk-")},
	{name: "key", find: regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`), hints: hint("xox")},
	{name: "key", find: regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`), hints: hint("AIza")},
	{name: "key", find: regexp.MustCompile(`\b[rsp]k_(?:live|test)_[0-9A-Za-z]{20,}`), hints: hint("k_live_", "k_test_")},
	{name: "key", find: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`), hints: hint("eyJ")},
	{name: "key", find: regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{24,}`), hints: hint("earer", "EARER")},
	{name: "email", find: regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`), hints: hint("@")},
	{name: "card", find: regexp.MustCompile(`\b\d(?:[ -]?\d){12,18}\b`), valid: luhn, scan: longRunOfDigits},
}

// longRunOfDigits says whether a message has thirteen digits in a row, allowing a single space or hyphen between two of them, which
// is the least a card number is.
func longRunOfDigits(message []byte) bool {
	run := 0
	for i := 0; i < len(message); i++ {
		switch c := message[i]; {
		case c >= '0' && c <= '9':
			if run++; run >= 13 {
				return true
			}
		case (c == ' ' || c == '-') && run > 0 && i+1 < len(message) && message[i+1] >= '0' && message[i+1] <= '9':
		default:
			run = 0
		}
	}
	return false
}

// Names is every kind this knows.
func Names() []string {
	var names []string
	for _, kind := range kinds {
		if !slices.Contains(names, kind.name) {
			names = append(names, kind.name)
		}
	}
	return names
}

// Redactor masks the kinds it was made with.
type Redactor struct {
	rules []rule
}

// New is a redactor for the named kinds. A kind this does not know is an error: a misspelt one would otherwise mask nothing.
func New(names ...string) (*Redactor, error) {
	wanted := map[string]bool{}
	for _, name := range names {
		found := false
		for _, kind := range kinds {
			found = found || kind.name == name
		}
		if !found {
			return nil, fmt.Errorf("%q is not a kind of data this masks: it masks %s", name, strings.Join(Names(), ", "))
		}
		wanted[name] = true
	}
	redactor := &Redactor{}
	for _, kind := range kinds {
		if wanted[kind.name] {
			redactor.rules = append(redactor.rules, kind)
		}
	}
	return redactor, nil
}

// Kinds is what this masks, by name.
func (r *Redactor) Kinds() []string {
	if r == nil {
		return nil
	}
	var names []string
	for _, each := range r.rules {
		if !slices.Contains(names, each.name) {
			names = append(names, each.name)
		}
	}
	sort.Strings(names)
	return names
}

// On says whether this masks anything.
func (r *Redactor) On() bool { return r != nil && len(r.rules) > 0 }

// Text masks what it finds in plain text, and says how much. A nil redactor, or one that masks nothing, returns the text as it was.
func (r *Redactor) Text(text []byte) ([]byte, Counts) { return r.text(text, false) }

// text is Text, and with quote it writes each mask as a JSON string, which is how a number that is masked stays a value.
func (r *Redactor) text(text []byte, quote bool) ([]byte, Counts) {
	if !r.On() {
		return text, nil
	}
	var counts Counts
	for _, each := range r.rules {
		if !each.worthIn(text) {
			continue
		}
		mask := []byte("[redacted:" + each.name + "]")
		if quote {
			mask = []byte(`"` + string(mask) + `"`)
		}
		text = each.find.ReplaceAllFunc(text, func(match []byte) []byte {
			if each.valid != nil && !each.valid(string(match)) {
				return match
			}
			if counts == nil {
				counts = Counts{}
			}
			counts[each.name]++
			return mask
		})
	}
	return text, counts
}

// JSON masks what it finds in the strings of a JSON message, and says how much. A message that is not JSON is left as it is by the
// parts of it that are not in quotation marks, so a body that is plain text is masked with Text instead.
func (r *Redactor) JSON(message []byte) ([]byte, Counts) {
	if !r.On() {
		return message, nil
	}
	r = r.relevant(message)
	if !r.On() {
		return message, nil
	}
	var counts Counts
	var out []byte
	copied := 0
	numbers := r.only("card")
	// replace puts masked text where [from, to) was.
	replace := func(from, to int, masked []byte, found Counts) {
		if counts == nil {
			counts = Counts{}
		}
		counts.Add(found)
		out = append(out, message[copied:from]...)
		out = append(out, masked...)
		copied = to
	}
	for i := 0; i < len(message); {
		open := bytes.IndexByte(message[i:], '"')
		if open < 0 {
			if masked, found := numbers.text(message[i:], true); found != nil {
				replace(i, len(message), masked, found)
			}
			break
		}
		// What lies between strings is numbers and punctuation, and a card number written as a number is still one.
		if masked, found := numbers.text(message[i:i+open], true); found != nil {
			replace(i, i+open, masked, found)
		}
		start := i + open + 1
		end := closing(message, start)
		if end < 0 {
			break
		}
		literal := message[start:end]
		if replaced, found := r.string(literal); found != nil {
			replace(start, end, replaced, found)
		}
		i = end + 1
	}
	if out == nil {
		return message, counts
	}
	return append(out, message[copied:]...), counts
}

// only is this redactor limited to the kind named, or nothing when it does not mask it.
func (r *Redactor) only(name string) *Redactor {
	narrowed := &Redactor{}
	for _, each := range r.rules {
		if each.name == name {
			narrowed.rules = append(narrowed.rules, each)
		}
	}
	return narrowed
}

// relevant is this redactor limited to the patterns a message has any sign of.
func (r *Redactor) relevant(message []byte) *Redactor {
	narrowed := &Redactor{}
	for _, each := range r.rules {
		if each.worthIn(message) {
			narrowed.rules = append(narrowed.rules, each)
		}
	}
	return narrowed
}

// string masks one string's contents, as written between its quotation marks, and returns what to write there in its place, or
// nothing when nothing was masked.
func (r *Redactor) string(literal []byte) ([]byte, Counts) {
	if bytes.IndexByte(literal, '\\') < 0 {
		masked, found := r.Text(literal)
		return masked, found
	}
	var plain string
	if json.Unmarshal(append(append([]byte{'"'}, literal...), '"'), &plain) != nil {
		return nil, nil
	}
	masked, found := r.Text([]byte(plain))
	if found == nil {
		return nil, nil
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(string(masked)) != nil {
		return nil, nil
	}
	quoted := bytes.TrimSuffix(encoded.Bytes(), []byte("\n"))
	return quoted[1 : len(quoted)-1], found
}

// closing is where the string that begins at start ends, which is the quotation mark no backslash escapes.
func closing(message []byte, start int) int {
	for j := start; j < len(message); j++ {
		switch message[j] {
		case '\\':
			j++
		case '"':
			return j
		}
	}
	return -1
}

// Event masks one event of a stream of server-sent events: each data line that holds JSON as JSON, and any other as text.
func (r *Redactor) Event(event []byte) ([]byte, Counts) {
	if !r.On() {
		return event, nil
	}
	var counts Counts
	lines := bytes.Split(event, []byte("\n"))
	for i, line := range lines {
		payload, isData := bytes.CutPrefix(line, []byte("data:"))
		if !isData {
			continue
		}
		spaced := bytes.HasPrefix(payload, []byte(" "))
		payload = bytes.TrimPrefix(payload, []byte(" "))
		var masked []byte
		var found Counts
		if json.Valid(payload) {
			masked, found = r.JSON(payload)
		} else {
			masked, found = r.Text(payload)
		}
		if found == nil {
			continue
		}
		counts = counts.merged(found)
		prefix := "data:"
		if spaced {
			prefix = "data: "
		}
		lines[i] = append([]byte(prefix), masked...)
	}
	return bytes.Join(lines, []byte("\n")), counts
}

func (c Counts) merged(other Counts) Counts {
	if c == nil {
		c = Counts{}
	}
	c.Add(other)
	return c
}

// luhn is the check digit every card number carries: the digits, from the right, with every second one doubled, add up to a multiple
// of ten. A run of 13 to 19 digits that fails it is an identifier and not a card.
func luhn(match string) bool {
	digits := make([]int, 0, 19)
	for _, r := range match {
		if r >= '0' && r <= '9' {
			digits = append(digits, int(r-'0'))
		}
	}
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	sum := 0
	for i := range digits {
		d := digits[len(digits)-1-i]
		if i%2 == 1 {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}
