package web

import (
	"fmt"
	"io/fs"
	"path"
	"reflect"
	"slices"
	"testing"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/file"
	"github.com/dop251/goja/parser"
)

// vendoredScripts are the scripts static/PROVENANCE.md lists as somebody else's, which are not checked.
var vendoredScripts = []string{"htmx.min.js", "highlight.min.js", "centrifuge.js"}

// A scope that declares one function name twice keeps only the second: the first is replaced everywhere, and nothing says so. That is
// how the @ menu of a room stopped opening when polling added a second look() to the same function. No test loads the page scripts, so
// this parses each of ours and fails on a function declared twice in one scope.
func TestNoPageScriptDeclaresAFunctionTwiceInOneScope(t *testing.T) {
	scripts, err := fs.Glob(assets, "static/*.js")
	if err != nil || len(scripts) == 0 {
		t.Fatalf("no page scripts were found: %v", err)
	}
	for _, script := range scripts {
		if slices.Contains(vendoredScripts, path.Base(script)) {
			continue
		}
		source, err := fs.ReadFile(assets, script)
		if err != nil {
			t.Fatal(err)
		}
		repeats, err := repeatedFunctions(script, string(source))
		if err != nil {
			t.Errorf("%s does not parse: %v", script, err)
		}
		for _, repeat := range repeats {
			t.Errorf("%s", repeat)
		}
	}
}

func TestARepeatedFunctionIsFoundInOneScopeAndNotAcrossScopes(t *testing.T) {
	cases := map[string]int{
		"function a() {}\nfunction a() {}":                                               1,
		"function f() { function look() {} if (x) { function look() {} } }":              1,
		"function f() { function look() {} }\nfunction g() { function look() {} }":       0,
		"function f() { function look() {} var g = function () { function look() {} } }": 0,
		"var run = () => { function a() {} function a() {} }":                            1,
		"function a() {}\nvar b = function a() {}":                                       0,
	}
	for source, want := range cases {
		repeats, err := repeatedFunctions("case.js", source)
		if err != nil || len(repeats) != want {
			t.Errorf("%q: %d repeats (%v), want %d: %v", source, len(repeats), err, want, repeats)
		}
	}
}

// repeatedFunctions says which function names the script declares more than once in one scope, with the lines.
func repeatedFunctions(name, source string) ([]string, error) {
	files := &file.FileSet{}
	program, err := parser.ParseFile(files, name, source, 0)
	if err != nil {
		return nil, err
	}
	scan := &scopeScan{files: files, name: name}
	scan.walk(reflect.ValueOf(program), map[string]file.Idx{})
	return scan.repeats, nil
}

type scopeScan struct {
	files   *file.FileSet
	name    string
	repeats []string
}

// walk visits every node below value, which is the syntax tree of the script. A tree of some seventy node types is walked by its
// fields and not by a method for each, since the only nodes that matter are function declarations and where functions begin.
func (s *scopeScan) walk(value reflect.Value, declared map[string]file.Idx) {
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return
		}
		if function, ok := value.Interface().(*ast.FunctionDeclaration); ok {
			s.declare(function, declared)
		}
		s.enter(value, declared)
	case reflect.Slice:
		for i := range value.Len() {
			s.walk(value.Index(i), declared)
		}
	case reflect.Struct:
		if value.Type().PkgPath() != reflect.TypeFor[ast.Program]().PkgPath() {
			return
		}
		for i := range value.NumField() {
			// The parser lists each var again as a declaration of its scope, which would visit its initializer a second time.
			if value.Type().Field(i).Name != "DeclarationList" {
				s.walk(value.Field(i), declared)
			}
		}
	}
}

// enter walks the node a pointer or interface holds: in a scope of its own when it is a function, and in the one it is in otherwise.
func (s *scopeScan) enter(value reflect.Value, declared map[string]file.Idx) {
	target := value
	if value.Kind() == reflect.Interface {
		target = value.Elem()
	}
	if target.Kind() != reflect.Pointer {
		return
	}
	switch target.Interface().(type) {
	case *ast.FunctionLiteral, *ast.ArrowFunctionLiteral:
		declared = map[string]file.Idx{}
	}
	s.walk(target.Elem(), declared)
}

func (s *scopeScan) declare(function *ast.FunctionDeclaration, declared map[string]file.Idx) {
	if function.Function == nil || function.Function.Name == nil {
		return
	}
	name := string(function.Function.Name.Name)
	here := function.Function.Name.Idx
	if first, again := declared[name]; again {
		s.repeats = append(s.repeats, fmt.Sprintf("%s declares the function %s at line %d and again at line %d in the same scope: the second replaces the first everywhere",
			s.name, name, s.files.Position(first).Line, s.files.Position(here).Line))
		return
	}
	declared[name] = here
}
