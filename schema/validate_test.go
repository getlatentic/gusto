package schema

import (
	"strings"
	"testing"
)

const person = `{
  "type": "object",
  "required": ["name", "age"],
  "additionalProperties": false,
  "properties": {
    "version": {"const": 1},
    "name": {"type": "string", "description": "an annotation, not a rule"},
    "age": {"type": "integer"},
    "height": {"type": ["number", "null"]},
    "label": {"enum": ["public", "internal"]},
    "tags": {"type": "array", "items": {"type": "string"}},
    "extra": {"type": "object", "additionalProperties": {"type": "boolean"}}
  }
}`

func TestADocumentThatMatchesPasses(t *testing.T) {
	document := `{"version": 1.0, "name": "a", "age": 3, "height": null, "label": "public",
		"tags": ["x"], "extra": {"any": true}}`
	if err := Validate([]byte(person), []byte(document)); err != nil {
		t.Fatal(err)
	}
}

func TestEachRuleNamesWhereItFailed(t *testing.T) {
	for _, tc := range []struct{ document, want string }{
		{`{"age": 3}`, `/: "name" is required`},
		{`{"name": 1, "age": 3}`, `/name: want string, got integer`},
		{`{"name": "a", "age": 1.5}`, `/age: want integer, got number`},
		{`{"name": "a", "age": 3, "height": "tall"}`, `/height: want number or null, got string`},
		{`{"name": "a", "age": 3, "label": "secret"}`, `/label: "secret" is not one of ["public","internal"]`},
		{`{"name": "a", "age": 3, "version": 2}`, `/version: want 1, got 2`},
		{`{"name": "a", "age": 3, "tags": ["x", 2]}`, `/tags/1: want string, got integer`},
		{`{"name": "a", "age": 3, "colour": "red"}`, `/: "colour" is not a field of this format`},
		{`{"name": "a", "age": 3, "extra": {"on": "yes"}}`, `/extra/on: want boolean, got string`},
		{`[]`, `/: want object, got array`},
	} {
		err := Validate([]byte(person), []byte(tc.document))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want %q, got %v", tc.document, tc.want, err)
		}
	}
}

func TestEveryProblemIsReportedNotJustTheFirst(t *testing.T) {
	err := Validate([]byte(person), []byte(`{"name": 1, "age": "old"}`))
	if err == nil || strings.Count(err.Error(), "\n") != 1 {
		t.Fatalf("want two problems, got %v", err)
	}
}

func TestAKeywordTheValidatorDoesNotImplementIsRefused(t *testing.T) {
	schema := `{"type": "object", "properties": {"age": {"type": "integer", "minimum": 0}}}`
	err := Validate([]byte(schema), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), `/properties/age: the schema uses "minimum"`) {
		t.Fatalf("a rule it cannot check must not pass silently, got %v", err)
	}
}

func TestLargeIntegersKeepTheirDigits(t *testing.T) {
	schema := `{"const": 9007199254740993}`
	if err := Validate([]byte(schema), []byte(`9007199254740992`)); err == nil {
		t.Fatal("two integers past a float's precision must still differ")
	}
}

func TestADocumentThatIsNotJSONIsRefused(t *testing.T) {
	if err := Validate([]byte(person), []byte(`{"name": `)); err == nil {
		t.Fatal("want an error")
	}
	if err := Validate([]byte(person), []byte(`{} {}`)); err == nil {
		t.Fatal("two values are not one document")
	}
}
