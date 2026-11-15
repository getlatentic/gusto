package web

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// Every kind of event the changelog writes is said in words, so a new kind never reaches the page as its raw name.
func TestEveryKindOfEventIsSaidInWords(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "../changelog/log.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	kinds := 0
	for _, decl := range file.Decls {
		block, ok := decl.(*ast.GenDecl)
		if !ok || block.Tok != token.CONST || block.Doc == nil || block.Doc.Text() != "Kinds of event.\n" {
			continue
		}
		for _, spec := range block.Specs {
			for _, value := range spec.(*ast.ValueSpec).Values {
				kind, err := strconv.Unquote(value.(*ast.BasicLit).Value)
				if err != nil {
					t.Fatal(err)
				}
				kinds++
				if _, said := eventWords[kind]; !said {
					t.Errorf("the event %q has no words", kind)
				}
			}
		}
	}
	if kinds == 0 {
		t.Fatal("no kinds of event were found in the changelog")
	}
}
