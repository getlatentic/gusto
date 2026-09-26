package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

// Validate checks a JSON document against a JSON Schema and names every place
// it does not match.
//
// It implements the part of draft 2020-12 the two schemas here use: type,
// required, properties, additionalProperties, enum, const and items. A schema
// that uses any other keyword is refused rather than half-checked, because a
// validator that skips what it does not understand passes documents it never
// looked at. Annotations such as description and default carry no rule and are
// allowed.
func Validate(schema, document []byte) error {
	rules, err := decode(schema)
	if err != nil {
		return fmt.Errorf("the schema is not JSON: %w", err)
	}
	if unknown := unsupported(rules, ""); len(unknown) > 0 {
		return errors.New(strings.Join(unknown, "\n"))
	}
	value, err := decode(document)
	if err != nil {
		return fmt.Errorf("the document is not JSON: %w", err)
	}
	if problems := check(rules, value, ""); len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

// decode keeps numbers as written, so an integer is told from 1.5 exactly and a
// large one does not lose digits in a float.
func decode(text []byte) (any, error) {
	reader := json.NewDecoder(bytes.NewReader(text))
	reader.UseNumber()
	var value any
	if err := reader.Decode(&value); err != nil {
		return nil, err
	}
	if reader.More() {
		return nil, errors.New("more than one value")
	}
	return value, nil
}

// keyword checks one rule of a schema against a value at a JSON pointer.
type keyword func(schema map[string]any, rule, value any, at string) []string

// keywords is filled in init because the checks that hold a subschema recurse
// through check, which reads this map, and Go refuses that cycle in a literal.
var keywords map[string]keyword

func init() {
	keywords = map[string]keyword{
		"type":                 checkType,
		"enum":                 checkEnum,
		"const":                checkConst,
		"required":             checkRequired,
		"properties":           checkProperties,
		"additionalProperties": checkAdditional,
		"items":                checkItems,
	}
}

var annotations = map[string]bool{
	"$schema": true, "$id": true, "$comment": true,
	"title": true, "description": true, "default": true, "examples": true,
}

func check(schema, value any, at string) []string {
	rules, ok := schema.(map[string]any)
	if !ok {
		return []string{fmt.Sprintf("%s: the schema here is not an object", where(at))}
	}
	var problems []string
	for _, name := range sortedKeys(rules) {
		if apply, ok := keywords[name]; ok {
			problems = append(problems, apply(rules, rules[name], value, at)...)
		}
	}
	return problems
}

// unsupported walks the whole schema up front, so a keyword inside a branch no
// document happened to reach is still refused.
func unsupported(schema any, at string) []string {
	rules, ok := schema.(map[string]any)
	if !ok {
		return nil
	}
	var found []string
	for _, name := range sortedKeys(rules) {
		if keywords[name] == nil && !annotations[name] {
			found = append(found, fmt.Sprintf("%s: the schema uses %q, which this validator does not implement", where(at), name))
		}
	}
	for _, name := range sortedKeys(object(rules["properties"])) {
		found = append(found, unsupported(object(rules["properties"])[name], at+"/properties/"+name)...)
	}
	found = append(found, unsupported(rules["items"], at+"/items")...)
	return append(found, unsupported(rules["additionalProperties"], at+"/additionalProperties")...)
}

func where(at string) string {
	if at == "" {
		return "/"
	}
	return at
}

func object(value any) map[string]any {
	found, _ := value.(map[string]any)
	return found
}

func sortedKeys(values map[string]any) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// equal compares two JSON values the way JSON Schema does: numbers by value, so
// 1 and 1.0 are the same, and everything else by structure.
func equal(a, b any) bool {
	switch a := a.(type) {
	case json.Number:
		b, ok := b.(json.Number)
		return ok && sameNumber(a, b)
	case []any:
		b, ok := b.([]any)
		return ok && sameItems(a, b)
	case map[string]any:
		b, ok := b.(map[string]any)
		return ok && sameMembers(a, b)
	default:
		return a == b
	}
}

func sameNumber(a, b json.Number) bool {
	x, okX := new(big.Rat).SetString(a.String())
	y, okY := new(big.Rat).SetString(b.String())
	return okX && okY && x.Cmp(y) == 0
}

func sameItems(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func sameMembers(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for name, value := range a {
		other, ok := b[name]
		if !ok || !equal(value, other) {
			return false
		}
	}
	return true
}
