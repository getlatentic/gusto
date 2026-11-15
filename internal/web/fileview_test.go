package web

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/instance"
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
	return site, id, site.server.instances.AgentFolder(id)
}

// fileJSON is the file viewer's data, as web/src/pages/files/types.ts names it.
type fileJSON struct {
	File                                viewedFile
	Crumbs                              []pageCrumb
	Prose, Back, BackWord, Save, Source string
}

func viewed(t *testing.T, site *site, id, name string) fileJSON {
	t.Helper()
	var page fileJSON
	site.data(t, "/instances/"+id+"/file?path="+url.QueryEscape(name), &page)
	return page
}

// A file is shown in the way its kind is read, and never as the bytes of one that is not text.
func TestAFileIsShownInTheWayItsKindIsRead(t *testing.T) {
	site, id, folder := viewerSite(t)
	os.WriteFile(filepath.Join(folder, "notes.md"), []byte("# Title\n\nSome *words*.\n\n> quoted\n"), 0o644)
	os.WriteFile(filepath.Join(folder, "data.csv"), []byte("name,total\nada,3\n"), 0o644)
	os.WriteFile(filepath.Join(folder, "pic.png"), []byte("\x89PNG\r\n\x1a\n\x00\x00"), 0o644)
	os.WriteFile(filepath.Join(folder, "paper.pdf"), []byte("%PDF-1.4\n"), 0o644)
	os.WriteFile(filepath.Join(folder, "game.html"), []byte("<script>alert(1)</script>"), 0o644)
	os.WriteFile(filepath.Join(folder, "blob.bin"), []byte("a\x00b\x00"), 0o644)
	os.WriteFile(filepath.Join(folder, "main.go"), []byte("package main\n"), 0o644)

	raw := "/instances/" + id + "/raw?path="
	for name, kind := range map[string]fileKind{"notes.md": kindMarkdown, "data.csv": kindTable, "pic.png": kindImage, "paper.pdf": kindPDF,
		"game.html": kindHTML, "blob.bin": kindBinary, "main.go": kindText} {
		page := viewed(t, site, id, name)
		if page.File.Kind != kind || page.File.Raw != raw+url.QueryEscape(name) || page.File.Download != page.File.Raw+"&download=1" {
			t.Errorf("%s is shown as %q from %q, %q", name, page.File.Kind, page.File.Raw, page.File.Download)
		}
	}
	if notes := viewed(t, site, id, "notes.md"); !strings.Contains(notes.Prose, "<blockquote>") || !strings.Contains(notes.Prose, "<em>words</em>") {
		t.Errorf("the Markdown is not drawn: %q", notes.Prose)
	}
	if table := viewed(t, site, id, "data.csv"); !reflect.DeepEqual(table.File.Rows, [][]string{{"name", "total"}, {"ada", "3"}}) {
		t.Errorf("the table is %v", table.File.Rows)
	}
	if code := viewed(t, site, id, "main.go"); code.File.Language != "go" || code.File.Text != "package main\n" || code.File.Lines != 1 {
		t.Errorf("the code is %q in %q with %d lines", code.File.Text, code.File.Language, code.File.Lines)
	}
	if page := viewed(t, site, id, "game.html"); page.Source != "/instances/"+id+"/file?path=game.html&source=1" || page.File.Text != "" {
		t.Errorf("a web page does not offer its source, or is read as text: %+v", page)
	}
	var source fileJSON
	site.data(t, "/instances/"+id+"/file?path=game.html&source=1", &source)
	if source.File.Kind != kindText || source.File.Language != "xml" || source.File.Text != "<script>alert(1)</script>" {
		t.Errorf("the source of a page is not shown as text: %+v", source.File)
	}
	if blob := viewed(t, site, id, "blob.bin"); blob.File.Text != "" || blob.Save != "" {
		t.Errorf("a file that is not text is read as text: %+v", blob.File)
	}
}

// Word, PowerPoint and Excel files are read for the words in them.
func TestAnOfficeFileIsReadForItsWords(t *testing.T) {
	site, id, folder := viewerSite(t)
	zipOf(t, filepath.Join(folder, "plan.docx"), map[string]string{"word/document.xml": `<w:document xmlns:w="w"><w:body>
		<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>The plan</w:t></w:r></w:p>
		<w:p><w:r><w:t>First we </w:t></w:r><w:r><w:t>begin.</w:t></w:r></w:p></w:body></w:document>`})
	zipOf(t, filepath.Join(folder, "deck.pptx"), map[string]string{
		"ppt/slides/slide2.xml": `<p:sld xmlns:p="p" xmlns:a="a"><a:p><a:r><a:t>Second slide</a:t></a:r></a:p></p:sld>`,
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="p" xmlns:a="a"><a:p><a:r><a:t>Cover</a:t></a:r></a:p><a:p><a:r><a:t>Govern AI</a:t></a:r></a:p></p:sld>`})
	zipOf(t, filepath.Join(folder, "sheet.xlsx"), map[string]string{
		"xl/sharedStrings.xml":     `<sst><si><t>Name</t></si><si><t>Ada</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1"><v>Total</v></c></row><row r="2"><c r="A2" t="s"><v>1</v></c><c r="B2"><v>7</v></c></row></sheetData></worksheet>`})

	doc := viewed(t, site, id, "plan.docx").File
	if doc.Kind != kindOffice || len(doc.Parts) != 1 || !reflect.DeepEqual(doc.Parts[0].Lines, []officeLine{{Text: "The plan", Heading: 1}, {Text: "First we begin."}}) {
		t.Errorf("the document: %+v", doc.Parts)
	}
	deck := viewed(t, site, id, "deck.pptx").File
	if len(deck.Parts) != 2 || deck.Parts[0].Title != "Slide 1" || deck.Parts[0].Lines[0].Text != "Cover" || deck.Parts[0].Lines[1].Text != "Govern AI" || deck.Parts[1].Lines[0].Text != "Second slide" {
		t.Errorf("the slides are not in order or lack their words: %+v", deck.Parts)
	}
	sheet := viewed(t, site, id, "sheet.xlsx").File
	if len(sheet.Parts) != 1 || !reflect.DeepEqual(sheet.Parts[0].Rows, [][]string{{"Name", "Total"}, {"Ada", "7"}}) {
		t.Errorf("the sheet: %+v", sheet.Parts)
	}
	os.WriteFile(filepath.Join(folder, "broken.docx"), []byte("not a zip"), 0o644)
	if page := viewed(t, site, id, "broken.docx"); page.File.Kind != kindBinary {
		t.Errorf("a file that is not a document was shown as one: %q", page.File.Kind)
	}
}

// What the agent wrote is served so that it cannot act as this site: a page runs sealed, a picture is only a picture, and what a browser
// would not show is a download. Nothing outside the folder is served.
func TestWhatTheAgentWroteIsServedSealed(t *testing.T) {
	site, id, folder := viewerSite(t)
	os.WriteFile(filepath.Join(folder, "game.html"), []byte("<h1>x</h1>"), 0o644)
	os.WriteFile(filepath.Join(folder, "a.svg"), []byte("<svg/>"), 0o644)
	os.WriteFile(filepath.Join(folder, "pack.zip"), []byte("PK\x03\x04\x00\x00"), 0o644)
	os.WriteFile(filepath.Join(folder, "notes.md"), []byte("hi"), 0o644)
	raw := func(path string) *httptest.ResponseRecorder {
		return site.ask(t, "GET", "/instances/"+id+"/raw?path="+url.QueryEscape(path), nil)
	}

	page := raw("game.html")
	if policy := page.Header().Get("Content-Security-Policy"); !strings.Contains(policy, "sandbox allow-scripts allow-forms ") || !strings.Contains(policy, "form-action 'none'") || !strings.Contains(policy, "default-src 'none'") || strings.Contains(policy, "connect-src") && !strings.Contains(policy, "default-src 'none'") {
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
// The Files pages are the React app's, whose rail is checked where it is built.
func TestTheListOfChatsIsBesideChatsAndTheRailLeadsToFiles(t *testing.T) {
	site, id, _ := viewerSite(t)
	site.ask(t, "POST", "/instances/"+id+"/threads/1/say", url.Values{"said": {"hello"}})
	files := site.ask(t, "GET", "/files", nil)
	if files.Code != http.StatusOK || !strings.Contains(files.Body.String(), `<div id="root">`) {
		t.Fatalf("the files page answered %d without the app", files.Code)
	}
	if _, listed := shellOf(t, site).Rail.project(id); !listed {
		t.Errorf("the side list beside chats does not carry %s", id)
	}
	for _, page := range []string{"/jobs", "/spend"} {
		body := site.ask(t, "GET", page, nil).Body.String()
		if !strings.Contains(body, `href="/files"`) {
			t.Errorf("%s does not carry the rail with Files in it", page)
		}
		if carries := strings.Contains(body, `href="/chat?at=`+id+`"`); carries {
			t.Errorf("%s carries the list of chats: %v", page, carries)
		}
		if strings.Contains(body, "hx-trigger=\"load\"") || strings.Contains(body, `href="/compare" data-tip`) {
			t.Errorf("%s loads the side list afterwards or still has Compare in it", page)
		}
	}
}

// The Files page is open at the place its address names: the top, with the person's projects, or a folder of one, with its trail.
// A place that is not there, or not theirs, is not found, and the way back is a page of this server or none.
func TestTheFilesPageIsOpenAtThePlaceItsAddressNames(t *testing.T) {
	site, id, folder := viewerSite(t)
	os.MkdirAll(filepath.Join(folder, "notes", "old"), 0o755)
	os.WriteFile(filepath.Join(folder, "notes", "plan.md"), []byte("plan"), 0o644)
	var page struct {
		Here shownFolders
		Back string
	}
	site.data(t, "/files", &page)
	if page.Here.At != "" || len(page.Here.Folders) != 1 || page.Here.Folders[0].At != id {
		t.Errorf("the top does not hold the person's project: %+v", page.Here)
	}
	site.data(t, "/files?at="+url.QueryEscape(id+"/notes")+"&from=/chat", &page)
	if page.Here.At != id+"/notes" || len(page.Here.Folders) != 1 || page.Here.Folders[0].Name != "old" || len(page.Here.Files) != 1 || page.Here.Files[0].Name != "plan.md" || page.Back != "/chat" {
		t.Errorf("the folder: %+v, back to %q", page.Here, page.Back)
	}
	if crumbs := page.Here.Crumbs; len(crumbs) != 3 || crumbs[1].At != id || crumbs[2] != (crumb{Name: "notes", At: id + "/notes"}) {
		t.Errorf("the trail is %+v", crumbs)
	}
	site.data(t, "/files?at="+id+"&from=https://elsewhere.test/", &page)
	if page.Back != "" {
		t.Errorf("the way back leads off this server, to %q", page.Back)
	}
	for _, at := range []string{id + "/missing", id + "/notes/plan.md", id + "/../..", "nobody"} {
		for _, path := range []string{"/files?at=", "/app/api/files?at="} {
			if got := site.ask(t, "GET", path+url.QueryEscape(at), nil); got.Code != http.StatusNotFound {
				t.Errorf("%s%s answered %d", path, at, got.Code)
			}
		}
	}
	if got := site.as(t, bob).ask(t, "GET", "/files?at="+id, nil); got.Code != http.StatusNotFound {
		t.Errorf("somebody else's folder answered %d", got.Code)
	}
}

// One agent's folder is the Files page open at it, with the way back to where the person came from.
func TestAnAgentsFolderIsTheFilesPageOpenAtIt(t *testing.T) {
	site, id, _ := viewerSite(t)
	moved := site.ask(t, "GET", "/instances/"+id+"/files?from=/instances/"+id+"/threads/1", nil)
	if want := "/files?at=" + id + "&from=" + url.QueryEscape("/instances/"+id+"/threads/1"); moved.Code != http.StatusSeeOther || moved.Header().Get("Location") != want {
		t.Errorf("the folder answered %d to %q", moved.Code, moved.Header().Get("Location"))
	}
	if gone := site.as(t, bob).ask(t, "GET", "/instances/"+id+"/files", nil); gone.Code != http.StatusNotFound {
		t.Errorf("somebody else's folder answered %d", gone.Code)
	}
}

// A file's page is the app's, under where the file is, with the way back the person came by. It is the person's alone, and no
// page may frame it: the file panel beside a conversation draws the file itself.
func TestAFilesPageIsWhereTheFileIsAndIsThePersonsAlone(t *testing.T) {
	site, id, folder := viewerSite(t)
	os.MkdirAll(filepath.Join(folder, "notes"), 0o755)
	os.WriteFile(filepath.Join(folder, "notes", "plan.md"), []byte("# The plan\n\nBegin here."), 0o644)
	var page fileJSON
	site.data(t, "/instances/"+id+"/file?path=notes/plan.md&from=/elsewhere", &page)
	agent := site.server.agentOf(mustInstance(t, site, id)).Name
	want := []pageCrumb{{"Files", "/files"}, {agent, "/files?at=" + id}, {"notes", "/files?at=" + url.QueryEscape(id+"/notes")}}
	if !reflect.DeepEqual(page.Crumbs, want) || page.Back != "/elsewhere" || page.File.Name != "plan.md" || !strings.Contains(page.Prose, "The plan") {
		t.Errorf("the file's page: %+v", page)
	}
	answer := site.ask(t, "GET", "/instances/"+id+"/file?path=notes/plan.md", nil)
	if answer.Code != http.StatusOK || answer.Header().Get("X-Frame-Options") != "DENY" || !strings.Contains(answer.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Errorf("the file's own page answered %d, or may be framed", answer.Code)
	}
	for _, path := range []string{"/instances/" + id + "/file?path=notes/plan.md", "/app/api/instances/" + id + "/file?path=notes/plan.md"} {
		if gone := site.as(t, bob).ask(t, "GET", path, nil); gone.Code != http.StatusNotFound {
			t.Errorf("somebody else's file at %s answered %d", path, gone.Code)
		}
	}
}

func mustInstance(t *testing.T, site *site, id string) instance.Instance {
	t.Helper()
	held, found := site.server.instances.Get(id)
	if !found {
		t.Fatalf("there is no %s", id)
	}
	return held
}
