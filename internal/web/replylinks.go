package web

import (
	"bytes"
	"html/template"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// What an agent says of a file it made is a link to it, by a path in its own folder: relative to the folder it was working in, or whole
// from the box's own root. Left as it is, a link opens a page that is not there. Here it opens the file in the agent's folder, a folder
// opens the list of files, and a link to something that is not in the folder is the words alone.

// workspaceMark replaces the box's path in a link, so that a path from the folder's root is told from one relative to where the agent
// was working.
const workspaceMark = "gusto-workspace:"

var hasScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// fileLinks draws the links of one reply of one agent's conversation.
type fileLinks struct {
	id, dir, back string
	workspace     string
}

// replyIn is a reply as HTML, with its links to the agent's files made to open them.
func (s *Server) replyIn(text, id, dir, back string) template.HTML {
	text = strings.ReplaceAll(text, "](/job/workspace/", "]("+workspaceMark)
	text = strings.ReplaceAll(text, "/job/workspace/", "")
	renderers := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			renderer.WithNodeRenderers(util.Prioritized(untrusted{}, 100), util.Prioritized(fileLinks{id: id, dir: dir, back: back, workspace: s.instances.Workspace(id)}, 99)),
		),
	)
	var drawn bytes.Buffer
	if err := renderers.Convert([]byte(text), &drawn); err != nil {
		return template.HTML("<pre>" + template.HTMLEscapeString(text) + "</pre>")
	}
	return template.HTML(drawn.String())
}

func (f fileLinks) RegisterFuncs(register renderer.NodeRendererFuncRegisterer) {
	register.Register(ast.KindLink, f.link)
}

func (f fileLinks) link(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	link := node.(*ast.Link)
	if !entering {
		if f.opened(node) {
			_, _ = w.WriteString("</a>")
		} else if f.isFile(string(link.Destination)) {
			_, _ = w.WriteString("</span>")
		} else {
			_, _ = w.WriteString("</a>")
		}
		return ast.WalkContinue, nil
	}
	destination := string(link.Destination)
	if address, ok := f.address(destination); ok {
		_, _ = w.WriteString(`<a href="` + template.HTMLEscapeString(address) + `">`)
		return ast.WalkContinue, nil
	}
	if f.isFile(destination) {
		_, _ = w.WriteString(`<span class="gone" title="This is not in the agent's folder">`)
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(`<a href="`)
	if !html.IsDangerousURL(link.Destination) {
		_, _ = w.Write(util.EscapeHTML(util.URLEscape(link.Destination, true)))
	}
	_, _ = w.WriteString(`" target="_blank" rel="noopener noreferrer"`)
	if link.Title != nil {
		_, _ = w.WriteString(` title="`)
		_, _ = w.Write(util.EscapeHTML(link.Title))
		_, _ = w.WriteString(`"`)
	}
	_, _ = w.WriteString(`>`)
	return ast.WalkContinue, nil
}

// opened says whether the link draws as a link to a file or folder that is there, which its closing has to match.
func (f fileLinks) opened(node ast.Node) bool {
	_, ok := f.address(string(node.(*ast.Link).Destination))
	return ok
}

// isFile says whether a destination is a path in the agent's folder, and not an address or a place on this site.
func (f fileLinks) isFile(destination string) bool {
	if strings.HasPrefix(destination, workspaceMark) {
		return true
	}
	return destination != "" && !strings.HasPrefix(destination, "/") && !strings.HasPrefix(destination, "#") && !hasScheme.MatchString(destination)
}

// address is where a link to the agent's file or folder goes, when it is one that is there.
func (f fileLinks) address(destination string) (string, bool) {
	if !f.isFile(destination) {
		return "", false
	}
	base := f.dir
	if rest, whole := strings.CutPrefix(destination, workspaceMark); whole {
		destination, base = rest, ""
	}
	if cut := strings.IndexAny(destination, "?#"); cut >= 0 {
		destination = destination[:cut]
	}
	joined := path.Join(base, destination)
	if joined == "." || joined == ".." || strings.HasPrefix(joined, "../") {
		return "", false
	}
	resolved, ok := resolveInFolder(f.workspace, "/"+joined)
	if !ok {
		return "", false
	}
	info, err := os.Stat(resolved)
	switch {
	case err != nil:
		return "", false
	case info.IsDir():
		return "/instances/" + f.id + "/files?from=" + url.QueryEscape(f.back), true
	}
	return "/instances/" + f.id + "/file?path=" + url.QueryEscape(joined) + "&from=" + url.QueryEscape(f.back), true
}
