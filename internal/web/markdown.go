package web

import (
	"bytes"
	"html/template"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// replies turns what a harness said into a page. A harness writes Markdown,
// and read as plain text its code blocks are a row of backticks.
//
// Everything it wrote is untrusted. goldmark drops links to javascript: and
// the like unless told otherwise, and it is not told otherwise. HTML the agent
// wrote is drawn as the text it is, so a reader sees what was said and none of
// it runs. Images are drawn as their address, not fetched: an image is a
// request from the reader's browser to wherever the agent chose, which is a way
// to learn who read the page and when. A line break the agent wrote stays one,
// because most of what a harness says is text, not Markdown.
var replies = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		html.WithHardWraps(),
		renderer.WithNodeRenderers(util.Prioritized(untrusted{}, 100)),
	),
)

// markdown is a reply as HTML, or the reply as escaped text when it cannot be
// drawn, which is still the whole of it.
func markdown(text string) template.HTML {
	var drawn bytes.Buffer
	if err := replies.Convert([]byte(text), &drawn); err != nil {
		return template.HTML("<pre>" + template.HTMLEscapeString(text) + "</pre>")
	}
	return template.HTML(drawn.String())
}

// untrusted draws what an agent wrote that a browser would otherwise act on.
type untrusted struct{}

func (untrusted) RegisterFuncs(register renderer.NodeRendererFuncRegisterer) {
	register.Register(ast.KindImage, image)
	register.Register(ast.KindRawHTML, asText)
	register.Register(ast.KindHTMLBlock, asText)
}

func image(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(`<span class="image">[image: `)
	_, _ = w.WriteString(template.HTMLEscapeString(string(node.(*ast.Image).Destination)))
	_, _ = w.WriteString(`]</span>`)
	return ast.WalkSkipChildren, nil
}

// asText writes raw HTML escaped. An inline piece keeps its segments; a block
// keeps its lines, and its closing line when it has one.
func asText(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	var said bytes.Buffer
	switch held := node.(type) {
	case *ast.RawHTML:
		for i := 0; i < held.Segments.Len(); i++ {
			segment := held.Segments.At(i)
			said.Write(segment.Value(source))
		}
		_, _ = w.WriteString(template.HTMLEscapeString(said.String()))
		return ast.WalkSkipChildren, nil
	case *ast.HTMLBlock:
		for i := 0; i < held.Lines().Len(); i++ {
			line := held.Lines().At(i)
			said.Write(line.Value(source))
		}
		if held.HasClosure() {
			closing := held.ClosureLine
			said.Write(closing.Value(source))
		}
		_, _ = w.WriteString("<p>" + template.HTMLEscapeString(said.String()) + "</p>\n")
	}
	return ast.WalkSkipChildren, nil
}
