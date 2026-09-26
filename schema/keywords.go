package schema

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

func checkType(_ map[string]any, rule, value any, at string) []string {
	allowed := names(rule)
	for _, name := range allowed {
		if is(name, value) {
			return nil
		}
	}
	return []string{fmt.Sprintf("%s: want %s, got %s", where(at), strings.Join(allowed, " or "), kind(value))}
}

// names reads a type rule, which is one name or a list of them.
func names(rule any) []string {
	if one, ok := rule.(string); ok {
		return []string{one}
	}
	list, _ := rule.([]any)
	found := make([]string, 0, len(list))
	for _, item := range list {
		if name, ok := item.(string); ok {
			found = append(found, name)
		}
	}
	return found
}

func is(name string, value any) bool {
	if name == "integer" {
		return integral(value)
	}
	return kind(value) == name || (name == "number" && kind(value) == "integer")
}

// kind names a decoded JSON value by its JSON Schema type, calling a number
// with no fractional part an integer as the draft does.
func kind(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	if integral(value) {
		return "integer"
	}
	return "number"
}

func integral(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	exact, ok := new(big.Rat).SetString(number.String())
	return ok && exact.IsInt()
}

func checkEnum(_ map[string]any, rule, value any, at string) []string {
	options, _ := rule.([]any)
	for _, option := range options {
		if equal(option, value) {
			return nil
		}
	}
	return []string{fmt.Sprintf("%s: %s is not one of %s", where(at), show(value), show(rule))}
}

func checkConst(_ map[string]any, rule, value any, at string) []string {
	if equal(rule, value) {
		return nil
	}
	return []string{fmt.Sprintf("%s: want %s, got %s", where(at), show(rule), show(value))}
}

func checkRequired(_ map[string]any, rule, value any, at string) []string {
	members, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	wanted, _ := rule.([]any)
	var problems []string
	for _, item := range wanted {
		name, _ := item.(string)
		if _, present := members[name]; !present {
			problems = append(problems, fmt.Sprintf("%s: %q is required", where(at), name))
		}
	}
	return problems
}

func checkProperties(_ map[string]any, rule, value any, at string) []string {
	members, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	declared := object(rule)
	var problems []string
	for _, name := range sortedKeys(members) {
		if schema, known := declared[name]; known {
			problems = append(problems, check(schema, members[name], at+"/"+name)...)
		}
	}
	return problems
}

// checkAdditional applies to the members properties does not declare. False
// refuses them, true allows them, and a schema checks each.
func checkAdditional(schema map[string]any, rule, value any, at string) []string {
	members, ok := value.(map[string]any)
	if !ok || rule == true {
		return nil
	}
	declared := object(schema["properties"])
	var problems []string
	for _, name := range sortedKeys(members) {
		if _, known := declared[name]; known {
			continue
		}
		problems = append(problems, additional(rule, members[name], at, name)...)
	}
	return problems
}

func additional(rule, value any, at, name string) []string {
	if rule == false {
		return []string{fmt.Sprintf("%s: %q is not a field of this format", where(at), name)}
	}
	return check(rule, value, at+"/"+name)
}

func checkItems(_ map[string]any, rule, value any, at string) []string {
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	var problems []string
	for i, item := range list {
		problems = append(problems, check(rule, item, fmt.Sprintf("%s/%d", at, i))...)
	}
	return problems
}

func show(value any) string {
	text, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(text)
}
