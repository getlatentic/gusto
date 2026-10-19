package ship_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/ship"
)

// The agent opens a pull request and never merges it. These tests hold that in
// three places: what reached the forge, what the forge can be asked, and every
// string the package could send to GitHub or to git.

func TestShippingSendsTheForgeOneRequestAndItOpensAPullRequest(t *testing.T) {
	dir, _ := finished(t, "yes")
	fake := &forge{}
	shipped(t, dir, fake)
	requests := fake.requests()
	if len(requests) != 1 || requests[0].Method != http.MethodPost || requests[0].Path != "/repos/o/r/pulls" {
		t.Fatalf("want exactly one POST /repos/o/r/pulls, got %v", requests)
	}
}

func TestTheForgeCanOnlyOpen(t *testing.T) {
	for _, kind := range []reflect.Type{reflect.TypeFor[ship.Forge](), reflect.TypeFor[ship.GitHub](), reflect.TypeFor[*ship.GitHub]()} {
		if kind.NumMethod() != 1 || kind.Method(0).Name != "Open" {
			t.Fatalf("%s must have Open and nothing else", kind)
		}
	}
}

func TestNoStringInThePackageAsksForAMerge(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil || len(sources) == 0 {
		t.Fatalf("the package's own sources should be here: %v", err)
	}
	files := token.NewFileSet()
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(files, source, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if ok && literal.Kind == token.STRING && strings.Contains(strings.ToLower(literal.Value), "merge") {
				t.Errorf("%s: %s", files.Position(literal.Pos()), literal.Value)
			}
			return true
		})
	}
}
