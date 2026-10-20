package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// The pages a person moves between, rendered as the server sends them, before any script has run.
func a11yPages(t *testing.T) map[string]*html.Node {
	t.Helper()
	site := open(t)
	site.signIn(t)
	conversation := startedThread(t, site)
	instance := conversation[:strings.Index(conversation, "/threads/")]
	pages := map[string]*html.Node{}
	for _, path := range []string{"/", "/chat", "/jobs", "/files", "/spend", "/new", "/admin", conversation, instance} {
		answer := site.ask(t, "GET", path, nil)
		if answer.Code != http.StatusOK {
			t.Fatalf("%s answered %d", path, answer.Code)
		}
		doc, err := html.Parse(answer.Body)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		pages[path] = doc
	}
	return pages
}

// Every link and button says what it is: a visible word, or a label when it is an icon alone.
func TestEveryControlHasAName(t *testing.T) {
	for path, doc := range a11yPages(t) {
		each(doc, func(n *html.Node) {
			if (n.Data == "a" && attr(n, "href") != "") || n.Data == "button" {
				if accessibleName(n) == "" {
					t.Errorf("%s: a %s with no name: %s", path, n.Data, render(n))
				}
			}
		})
	}
}

// Every field has a label, its own or one around it.
func TestEveryFieldHasALabel(t *testing.T) {
	for path, doc := range a11yPages(t) {
		labeled := map[string]bool{}
		each(doc, func(n *html.Node) {
			if n.Data == "label" && attr(n, "for") != "" {
				labeled[attr(n, "for")] = true
			}
		})
		each(doc, func(n *html.Node) {
			if n.Data != "input" && n.Data != "select" && n.Data != "textarea" {
				return
			}
			if kind := attr(n, "type"); kind == "hidden" || kind == "submit" {
				return
			}
			if attr(n, "aria-label") == "" && attr(n, "aria-labeledby") == "" && !labeled[attr(n, "id")] && !inside(n, "label") {
				t.Errorf("%s: a field with no label: %s", path, render(n))
			}
		})
	}
}

// One main a page, which the first control on the page skips to; every nav and aside is named; the page has a language, a title and at
// most one h1; what a control says it controls is on the page; a divider can be reached by the keyboard.
func TestEveryPageHasItsLandmarks(t *testing.T) {
	for path, doc := range a11yPages(t) {
		mains, h1s, ids := 0, 0, map[string]bool{}
		var first *html.Node
		each(doc, func(n *html.Node) {
			if id := attr(n, "id"); id != "" {
				ids[id] = true
			}
			switch n.Data {
			case "html":
				if attr(n, "lang") == "" {
					t.Errorf("%s: the page has no language", path)
				}
			case "title":
				if n.FirstChild == nil || strings.TrimSpace(n.FirstChild.Data) == "" {
					t.Errorf("%s: the page has no title", path)
				}
			case "main":
				mains++
			case "h1":
				h1s++
			case "nav", "aside":
				if attr(n, "aria-label") == "" {
					t.Errorf("%s: a %s with no name", path, n.Data)
				}
			case "a", "button", "input", "textarea", "select":
				if first == nil && inside(n, "body") {
					first = n
				}
			}
		})
		if mains != 1 {
			t.Errorf("%s: %d main landmarks", path, mains)
		}
		if h1s > 1 {
			t.Errorf("%s: %d h1 headings", path, h1s)
		}
		if first == nil || !slices.Contains(strings.Fields(attr(first, "class")), "skip") || !ids[strings.TrimPrefix(attr(first, "href"), "#")] {
			t.Errorf("%s: the first control is not a working skip link", path)
		}
		each(doc, func(n *html.Node) {
			for _, id := range strings.Fields(attr(n, "aria-controls")) {
				if !ids[id] {
					t.Errorf("%s: aria-controls names %q, which is not on the page", path, id)
				}
			}
			if attr(n, "role") == "separator" && (attr(n, "tabindex") != "0" || attr(n, "aria-label") == "") {
				t.Errorf("%s: a divider the keyboard cannot reach, or with no name", path)
			}
		})
	}
}

// A toggle says what it opens and whether it is open.
func TestTogglesSayWhetherTheyAreOpen(t *testing.T) {
	doc := a11yPages(t)["/chat"]
	for _, toggle := range []string{"data-panel-toggle", "data-look", "data-side-toggle"} {
		found := false
		each(doc, func(n *html.Node) {
			if hasAttr(n, toggle) {
				found = true
				if attr(n, "aria-expanded") == "" || attr(n, "aria-controls") == "" {
					t.Errorf("the %s button does not say what it opens or whether it is open", toggle)
				}
			}
		})
		if !found {
			t.Errorf("no %s button on the chat", toggle)
		}
	}
}

// While a turn runs, what it is doing is announced politely as it changes, the log of its steps is not read out again each time it is
// redrawn, and a region outside the running turn is there to say when the reply is in.
func TestARunningTurnIsAnnouncedPolitelyAndOnce(t *testing.T) {
	started := holding(t)
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	site.ask(t, "POST", where+"/say", url.Values{"said": {"a long piece of work"}})
	<-started
	page := site.ask(t, "GET", where, nil).Body.String()
	site.ask(t, "POST", "/jobs/"+regexpFirst(t, page, `/jobs/([0-9a-z-]+)/stop`)+"/stop", nil)
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{}
	each(doc, func(n *html.Node) {
		if attr(n, "aria-live") == "assertive" || attr(n, "role") == "alert" {
			t.Errorf("the running turn interrupts: %s", render(n))
		}
		if id := attr(n, "id"); id != "" {
			roles[id] = attr(n, "role") + attr(n, "aria-live")
		}
	})
	for id, want := range map[string]string{"doing": "status", "live": "", "turn-news": "status"} {
		if got, ok := roles[id]; !ok || got != want {
			t.Errorf("#%s is %q, want %q", id, got, want)
		}
	}
}

// A name cut short by its column is whole in the page's text, and in a title for whoever points at it.
func TestACutNameIsWholeInItsTitle(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	site.ask(t, "POST", where+"/say", url.Values{"said": {"a name long enough to be cut short by the side list"}})
	doc, err := html.Parse(site.ask(t, "GET", where, nil).Body)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	each(doc, func(n *html.Node) {
		cut := (n.Data == "h1" && inside(n, "main")) || (n.Data == "summary" && inside(n, "aside")) || (n.Data == "a" && inside(n, "ol") && inside(n, "aside"))
		if !cut {
			return
		}
		checked++
		name, title := strings.Join(strings.Fields(accessibleName(n)), " "), attr(n, "title")
		if title == "" || !(strings.HasPrefix(title, name) || strings.HasPrefix(name, title)) {
			t.Errorf("%q is cut with the title %q", name, title)
		}
	})
	if checked < 3 {
		t.Errorf("only %d cut names were found to check", checked)
	}
}

// A chat that could not start says why where it is read at once, linked to the box it is about, and the message is required.
func TestARefusedChatSaysWhyAtTheBox(t *testing.T) {
	site := open(t)
	site.signIn(t)
	doc, err := html.Parse(site.ask(t, "GET", "/chat?refused="+url.QueryEscape("That folder is not yours."), nil).Body)
	if err != nil {
		t.Fatal(err)
	}
	var box, why *html.Node
	each(doc, func(n *html.Node) {
		if n.Data == "textarea" && attr(n, "name") == "said" {
			box = n
		}
		if attr(n, "id") == "refused" {
			why = n
		}
	})
	if box == nil || why == nil {
		t.Fatal("the chat has no box or no reason")
	}
	if attr(why, "role") != "alert" || attr(why, "id") == "" || attr(box, "aria-describedby") != attr(why, "id") || !hasAttr(box, "required") {
		t.Errorf("the reason is not announced and linked to a required box: %s %s", render(why), render(box))
	}
}

func each(n *html.Node, visit func(*html.Node)) {
	if n.Type == html.ElementNode {
		visit(n)
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		each(child, visit)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func inside(n *html.Node, tag string) bool {
	for up := n.Parent; up != nil; up = up.Parent {
		if up.Type == html.ElementNode && up.Data == tag {
			return true
		}
	}
	return false
}

// accessibleName is the label a control is given, or else the text it holds that is not hidden from assistive technology.
func accessibleName(n *html.Node) string {
	if label := strings.TrimSpace(attr(n, "aria-label") + attr(n, "aria-labeledby")); label != "" {
		return label
	}
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && attr(node, "aria-hidden") == "true" {
			return
		}
		if node.Type == html.TextNode {
			text.WriteString(node.Data)
		}
		if node.Type == html.ElementNode && node.Data == "img" {
			text.WriteString(attr(node, "alt"))
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return strings.TrimSpace(text.String())
}

func render(n *html.Node) string {
	var out strings.Builder
	_ = html.Render(&out, n)
	if s := out.String(); len(s) > 200 {
		return s[:200]
	}
	return out.String()
}
