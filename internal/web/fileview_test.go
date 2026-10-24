package web

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func zipOf(t *testing.T, path string, parts map[string]string) {
	t.Helper()
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	archive := zip.NewWriter(out)
	for name, body := range parts {
		part, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write([]byte(body))
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}

func viewerSite(t *testing.T) (*site, string, string) {
	t.Helper()
	site := open(t)
	site.signIn(t)
	id := strings.TrimPrefix(startedInstance(t, site), "/instances/")
	return site, id, site.server.instances.Workspace(id)
}

func viewed(t *testing.T, site *site, id, name string) string {
	t.Helper()
	answer := site.ask(t, "GET", "/instances/"+id+"/file?path="+url.QueryEscape(name), nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("%s answered %d", name, answer.Code)
	}
	return answer.Body.String()
}

// A file is shown in the way its kind is read, and never as the bytes of one that is not text.
func TestAFileIsShownInTheWayItsKindIsRead(t *testing.T) {
	site, id, workspace := viewerSite(t)
	os.WriteFile(filepath.Join(workspace, "notes.md"), []byte("# Title\n\nSome *words*.\n\n> quoted\n"), 0o644)
	os.WriteFile(filepath.Join(workspace, "data.csv"), []byte("name,total\nada,3\n"), 0o644)
	os.WriteFile(filepath.Join(workspace, "pic.png"), []byte("\x89PNG\r\n\x1a\n\x00\x00"), 0o644)
	os.WriteFile(filepath.Join(workspace, "paper.pdf"), []byte("%PDF-1.4\n"), 0o644)
	os.WriteFile(filepath.Join(workspace, "game.html"), []byte("<script>alert(1)</script>"), 0o644)
	os.WriteFile(filepath.Join(workspace, "blob.bin"), []byte("a\x00b\x00"), 0o644)
	os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o644)

	for name, want := range map[string][]string{
		"notes.md":  {`<article class="prose `, "<blockquote>"},
		"data.csv":  {"<th>name</th>", "<td>ada</td>"},
		"pic.png":   {`src="/instances/` + id + `/raw?path=pic.png"`},
		"paper.pdf": {`<iframe class="`, `src="/instances/` + id + `/raw?path=paper.pdf"`},
		"game.html": {`sandbox="allow-scripts"`, "View source"},
		"blob.bin":  {"cannot be shown here", "Download it"},
		"main.go":   {`data-language="go"><code>package main`, "1 line"},
	} {
		page := viewed(t, site, id, name)
		for _, text := range want {
			if !strings.Contains(page, text) {
				t.Errorf("%s: the page lacks %q", name, text)
			}
		}
	}
	if source := site.ask(t, "GET", "/instances/"+id+"/file?path=game.html&source=1", nil).Body.String(); !strings.Contains(source, `data-language="xml">`) || strings.Contains(source, `sandbox="allow-scripts"`) {
		t.Error("the source of a page is not shown as text")
	}
}

// Word, PowerPoint and Excel files are read for the words in them.
func TestAnOfficeFileIsReadForItsWords(t *testing.T) {
	site, id, workspace := viewerSite(t)
	zipOf(t, filepath.Join(workspace, "plan.docx"), map[string]string{"word/document.xml": `<w:document xmlns:w="w"><w:body>
		<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>The plan</w:t></w:r></w:p>
		<w:p><w:r><w:t>First we </w:t></w:r><w:r><w:t>begin.</w:t></w:r></w:p></w:body></w:document>`})
	zipOf(t, filepath.Join(workspace, "deck.pptx"), map[string]string{
		"ppt/slides/slide2.xml": `<p:sld xmlns:p="p" xmlns:a="a"><a:p><a:r><a:t>Second slide</a:t></a:r></a:p></p:sld>`,
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="p" xmlns:a="a"><a:p><a:r><a:t>Cover</a:t></a:r></a:p><a:p><a:r><a:t>Govern AI</a:t></a:r></a:p></p:sld>`})
	zipOf(t, filepath.Join(workspace, "sheet.xlsx"), map[string]string{
		"xl/sharedStrings.xml":     `<sst><si><t>Name</t></si><si><t>Ada</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1"><v>Total</v></c></row><row r="2"><c r="A2" t="s"><v>1</v></c><c r="B2"><v>7</v></c></row></sheetData></worksheet>`})

	doc := viewed(t, site, id, "plan.docx")
	if !strings.Contains(doc, ">The plan</h3>") || !strings.Contains(doc, ">First we begin.</p>") || !strings.Contains(doc, "not how it looks") {
		t.Errorf("the document: %s", doc)
	}
	deck := viewed(t, site, id, "deck.pptx")
	if strings.Index(deck, "Cover") > strings.Index(deck, "Second slide") || !strings.Contains(deck, "Slide 1") || !strings.Contains(deck, "Govern AI") {
		t.Errorf("the slides are not in order or lack their words: %s", deck)
	}
	sheet := viewed(t, site, id, "sheet.xlsx")
	if !strings.Contains(sheet, "<td>Name</td><td>Total</td>") || !strings.Contains(sheet, "<td>Ada</td><td>7</td>") {
		t.Errorf("the sheet: %s", sheet)
	}
	os.WriteFile(filepath.Join(workspace, "broken.docx"), []byte("not a zip"), 0o644)
	if page := viewed(t, site, id, "broken.docx"); !strings.Contains(page, "cannot be shown here") {
		t.Error("a file that is not a document was shown as one")
	}
}

// What the agent wrote is served so that it cannot act as this site: a page runs sealed, a picture is only a picture, and what a browser
// would not show is a download. Nothing outside the folder is served.
func TestWhatTheAgentWroteIsServedSealed(t *testing.T) {
	site, id, workspace := viewerSite(t)
	os.WriteFile(filepath.Join(workspace, "game.html"), []byte("<h1>x</h1>"), 0o644)
	os.WriteFile(filepath.Join(workspace, "a.svg"), []byte("<svg/>"), 0o644)
	os.WriteFile(filepath.Join(workspace, "pack.zip"), []byte("PK\x03\x04\x00\x00"), 0o644)
	os.WriteFile(filepath.Join(workspace, "notes.md"), []byte("hi"), 0o644)
	raw := func(path string) *httptest.ResponseRecorder {
		return site.ask(t, "GET", "/instances/"+id+"/raw?path="+url.QueryEscape(path), nil)
	}

	page := raw("game.html")
	if policy := page.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "sandbox allow-scripts") || !strings.Contains(policy, "default-src 'none'") || strings.Contains(policy, "connect-src") && !strings.Contains(policy, "default-src 'none'") {
		t.Errorf("a page's policy is %q", policy)
	}
	if page.Header().Get("X-Frame-Options") != "" || !strings.Contains(page.Header().Get("Content-Security-Policy"), "frame-ancestors 'self'") || page.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(page.Header().Get("Content-Type"), "text/html") {
		t.Errorf("a page's headers are %v", page.Header())
	}
	if image := raw("a.svg"); !strings.HasPrefix(image.Header().Get("Content-Security-Policy"), "sandbox;") {
		t.Errorf("a picture is not sealed: %q", image.Header().Get("Content-Security-Policy"))
	}
	if download := raw("pack.zip"); !strings.HasPrefix(download.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("an archive is not a download: %q", download.Header().Get("Content-Disposition"))
	}
	if text := raw("notes.md"); !strings.HasPrefix(text.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("text is served as %q", text.Header().Get("Content-Type"))
	}
	for _, bad := range []string{"../../etc/passwd", "/etc/passwd", ".git/config", "nothing.txt"} {
		if got := raw(bad); got.Code != http.StatusNotFound {
			t.Errorf("%q answered %d", bad, got.Code)
		}
	}
}

// Files is one of the places the rail leads to, and the list of chats is beside chats alone: the other pages carry the rail and not the list.
func TestTheListOfChatsIsBesideChatsAndTheRailLeadsToFiles(t *testing.T) {
	site, id, _ := viewerSite(t)
	site.ask(t, "POST", "/instances/"+id+"/threads/1/say", url.Values{"said": {"hello"}})
	files := site.ask(t, "GET", "/files", nil)
	if files.Code != http.StatusOK || !strings.Contains(files.Body.String(), `id="files-app"`) {
		t.Fatalf("the files page answered %d", files.Code)
	}
	for _, page := range []string{"/files", "/jobs", "/spend", "/chat"} {
		body := site.ask(t, "GET", page, nil).Body.String()
		if !strings.Contains(body, `href="/files"`) {
			t.Errorf("%s does not carry the rail with Files in it", page)
		}
		if carries := strings.Contains(body, `href="/chat?at=`+id+`"`); carries != (page == "/chat") {
			t.Errorf("%s carries the list of chats: %v", page, carries)
		}
		if strings.Contains(body, "hx-trigger=\"load\"") || strings.Contains(body, `href="/compare" data-tip`) {
			t.Errorf("%s loads the side list afterwards or still has Compare in it", page)
		}
	}
}

// A file can be asked for as a tab beside a chat: its name, what can be done with it and the file, without the page around it.
func TestAFileIsGivenAsATabBesideAChatWithoutThePageAroundIt(t *testing.T) {
	site, id, workspace := viewerSite(t)
	os.WriteFile(filepath.Join(workspace, "plan.md"), []byte("# The plan\n\nBegin here."), 0o644)
	answer := site.ask(t, "GET", "/instances/"+id+"/file?path=plan.md&panel=1", nil)
	body := answer.Body.String()
	if answer.Code != http.StatusOK || !strings.Contains(body, "The plan") || !strings.Contains(body, "plan.md") || !strings.Contains(body, "Open as a page") {
		t.Fatalf("the tab: %d %s", answer.Code, body)
	}
	for _, page := range []string{"<html", "<body", `class="rail"`} {
		if strings.Contains(body, page) {
			t.Errorf("the tab carries the page around it (%s)", page)
		}
	}
	if gone := site.as(t, bob).ask(t, "GET", "/instances/"+id+"/file?path=plan.md&panel=1", nil); gone.Code != http.StatusNotFound {
		t.Errorf("somebody else's file as a tab answered %d", gone.Code)
	}
}

// A conversation carries the panel the files open in, and the dividers that change the widths.
func TestAConversationHasAPanelForFilesAndDividersToResize(t *testing.T) {
	site := open(t)
	site.signIn(t)
	page := site.ask(t, "GET", startedThread(t, site), nil).Body.String()
	for _, want := range []string{`id="file-panel"`, `id="file-panel-resize"`, `id="panel-resize"`, "/static/filepanel.js", "/static/resize.js"} {
		if !strings.Contains(page, want) {
			t.Errorf("the conversation does not carry %s", want)
		}
	}
}
