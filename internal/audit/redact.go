package audit

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// scrubber applies a policy's field and pattern rules to one file, noting
// every rule that changed something.
type scrubber struct {
	fields   map[string]bool
	patterns []named
	hit      map[string]bool
}

type named struct {
	name  string
	match *regexp.Regexp
}

// scrubberFor compiles a policy that check has already accepted.
func scrubberFor(policy Redact) *scrubber {
	s := &scrubber{fields: map[string]bool{}, hit: map[string]bool{}}
	for _, field := range policy.Fields {
		s.fields[field] = true
	}
	for _, pattern := range policy.Patterns {
		s.patterns = append(s.patterns, named{pattern.Name, regexp.MustCompile(pattern.Match)})
	}
	return s
}

// file returns the file's bytes with the rules applied and the rules that
// changed them. A file nothing matched comes back byte for byte, so it still
// matches its hash in the manifest.
func (s *scrubber) file(name string, text []byte) ([]byte, []string) {
	s.hit = map[string]bool{}
	var scrubbed []byte
	switch {
	case strings.HasSuffix(name, ".json"):
		scrubbed = s.document(text, true)
	case strings.HasSuffix(name, ".jsonl"):
		scrubbed = s.lines(text)
	default:
		scrubbed = []byte(s.text(string(text)))
	}
	if len(s.hit) == 0 {
		return text, nil
	}
	return scrubbed, s.rules()
}

func (s *scrubber) rules() []string {
	rules := make([]string, 0, len(s.hit))
	for rule := range s.hit {
		rules = append(rules, rule)
	}
	sort.Strings(rules)
	return rules
}

// lines treats each line as its own document. A line that is not JSON is
// still text, and is scrubbed as text.
func (s *scrubber) lines(text []byte) []byte {
	var out bytes.Buffer
	for _, line := range bytes.SplitAfter(text, []byte("\n")) {
		body := bytes.TrimSuffix(line, []byte("\n"))
		if len(body) > 0 {
			out.Write(s.document(body, false))
		}
		if len(body) < len(line) {
			out.WriteByte('\n')
		}
	}
	return out.Bytes()
}

// document scrubs values and never keys, so what remains is still the same
// shape of document a reader of that file expects.
func (s *scrubber) document(text []byte, indent bool) []byte {
	reader := json.NewDecoder(bytes.NewReader(text))
	reader.UseNumber()
	var value any
	if err := reader.Decode(&value); err != nil {
		return []byte(s.text(string(text)))
	}
	value = s.value(value)
	written, err := encode(value, indent)
	if err != nil {
		return []byte(s.text(string(text)))
	}
	return written
}

func (s *scrubber) value(value any) any {
	switch value := value.(type) {
	case map[string]any:
		for name, member := range value {
			if s.fields[name] {
				s.hit["field "+name] = true
				value[name] = "[redacted]"
				continue
			}
			value[name] = s.value(member)
		}
		return value
	case []any:
		for i, item := range value {
			value[i] = s.value(item)
		}
		return value
	case string:
		return s.text(value)
	}
	return value
}

func (s *scrubber) text(text string) string {
	for _, pattern := range s.patterns {
		replaced := pattern.match.ReplaceAllLiteralString(text, "[redacted: "+pattern.name+"]")
		if replaced != text {
			s.hit["pattern "+pattern.name] = true
			text = replaced
		}
	}
	return text
}

// encode writes JSON the way the rest of haas does, and without escaping the
// angle brackets a redaction marker never has but a transcript often does.
func encode(value any, indent bool) ([]byte, error) {
	var out bytes.Buffer
	writer := json.NewEncoder(&out)
	writer.SetEscapeHTML(false)
	if indent {
		writer.SetIndent("", "  ")
	}
	if err := writer.Encode(value); err != nil {
		return nil, err
	}
	if !indent {
		return bytes.TrimSuffix(out.Bytes(), []byte("\n")), nil
	}
	return out.Bytes(), nil
}
