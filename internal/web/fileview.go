package web

import (
	"encoding/csv"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// fileKind is what the viewer makes of a file: how it is shown.
type fileKind string

const (
	kindText     fileKind = "text"
	kindMarkdown fileKind = "markdown"
	kindImage    fileKind = "image"
	kindPDF      fileKind = "pdf"
	kindHTML     fileKind = "html"
	kindAudio    fileKind = "audio"
	kindVideo    fileKind = "video"
	kindOffice   fileKind = "office"
	kindTable    fileKind = "table"
	kindBinary   fileKind = "binary"
)

var kindByExtension = map[string]fileKind{
	".md": kindMarkdown, ".markdown": kindMarkdown,
	".png": kindImage, ".jpg": kindImage, ".jpeg": kindImage, ".gif": kindImage, ".webp": kindImage, ".svg": kindImage, ".bmp": kindImage, ".ico": kindImage, ".avif": kindImage,
	".pdf":  kindPDF,
	".html": kindHTML, ".htm": kindHTML,
	".mp3": kindAudio, ".wav": kindAudio, ".ogg": kindAudio, ".m4a": kindAudio,
	".mp4": kindVideo, ".webm": kindVideo, ".mov": kindVideo,
	".docx": kindOffice, ".pptx": kindOffice, ".xlsx": kindOffice,
	".csv": kindTable, ".tsv": kindTable,
}

// kindOf is how a file is shown, by its name, and for a name that says nothing by whether its first bytes read as text.
func kindOf(name string, head []byte) fileKind {
	if kind, known := kindByExtension[strings.ToLower(filepath.Ext(name))]; known {
		return kind
	}
	if len(head) == 0 {
		return kindText
	}
	for _, b := range head {
		if b == 0 {
			return kindBinary
		}
	}
	if utf8.Valid(head) || utf8.Valid(head[:len(head)-min(len(head), utf8.UTFMax)]) {
		return kindText
	}
	return kindBinary
}

// viewedFile is what the page for one file is drawn from.
type viewedFile struct {
	Name, Path, Dir string
	Kind            fileKind
	Size            string
	Modified        int64
	Raw, Download   string
	Text            string
	// Editable is a text file shown whole, which a person may change here: Source is all of it, and Hash what it was then.
	Editable     bool
	Source, Hash string
	// Language is what a code file is highlighted as, and Lines how many it has.
	Language string
	Lines    int
	Cut      bool
	Rows     [][]string
	Parts    []officePart
}

const tableRows = 300

// fill reads what the kind of file needs of it.
func (s *Server) fill(file *viewedFile, resolved string, size int64) {
	switch file.Kind {
	case kindText, kindMarkdown:
		text, _ := readSome(resolved, mostShown)
		file.Text, file.Cut = strings.ToValidUTF8(string(text), ""), size > mostShown
		file.Language, file.Lines = languageOf(file.Name), linesIn(file.Text)
	case kindTable:
		text, _ := readSome(resolved, mostShown)
		reader := csv.NewReader(strings.NewReader(string(text)))
		reader.FieldsPerRecord = -1
		reader.LazyQuotes = true
		if strings.EqualFold(filepath.Ext(file.Name), ".tsv") {
			reader.Comma = '\t'
		}
		for len(file.Rows) < tableRows {
			row, err := reader.Read()
			if err != nil {
				break
			}
			file.Rows = append(file.Rows, row)
		}
		file.Cut = size > mostShown
		if len(file.Rows) == 0 {
			file.Kind = kindText
		}
	case kindOffice:
		parts, ok := readOffice(resolved, strings.ToLower(filepath.Ext(file.Name)))
		if !ok {
			file.Kind = kindBinary
			return
		}
		file.Parts = parts
	}
}

// folderRaw gives a file of an agent's folder as it is, for the page to show in a frame, a picture or a player, or for the person to
// keep. What the agent wrote is not trusted: a page it made runs in a frame that gives it nothing of this site and no way to send anything
// out, and anything the browser does not know how to show is handed over to be saved.
func (s *Server) folderRaw(w http.ResponseWriter, r *http.Request) {
	held, found := s.sharedOr404(w, r)
	if !found {
		return
	}
	asked := filepath.Clean("/" + r.URL.Query().Get("path"))
	resolved, ok := resolveInFolder(s.instances.AgentFolder(held.ID), asked)
	if !ok {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(resolved)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	name := filepath.Base(asked)
	head := make([]byte, 8<<10)
	read, _ := file.Read(head)
	if _, err := file.Seek(0, 0); err != nil {
		http.NotFound(w, r)
		return
	}
	kind := kindOf(name, head[:read])
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	// The policy says who may frame it. X-Frame-Options is left off because a browser reads it against a sealed frame as a stranger.
	h.Del("X-Frame-Options")
	h.Set("Content-Type", rawType(name, kind))
	h.Set("Content-Security-Policy", rawPolicy(kind))
	if r.URL.Query().Get("download") != "" || !shownInline(kind) {
		h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func shownInline(kind fileKind) bool {
	switch kind {
	case kindImage, kindPDF, kindHTML, kindAudio, kindVideo, kindText, kindMarkdown, kindTable:
		return true
	}
	return false
}

// rawType is the type a file is served as. A page and a picture are what they say they are; whatever else is text is plain, so that it is
// read and never run.
func rawType(name string, kind fileKind) string {
	switch kind {
	case kindText, kindMarkdown, kindTable:
		return "text/plain; charset=utf-8"
	case kindHTML:
		return "text/html; charset=utf-8"
	}
	if known := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); known != "" {
		return known
	}
	return "application/octet-stream"
}

// rawPolicy is what the browser is told a file may do. A page runs its own scripts and styles and may fetch nothing, and a picture may be
// nothing but a picture. A page's forms are allowed because a browser fires no submit event in a sandbox without them, and most
// pages an agent writes are driven by their forms; form-action 'none' still keeps a form from sending anything anywhere.
func rawPolicy(kind fileKind) string {
	switch kind {
	case kindHTML:
		return "sandbox allow-scripts allow-forms allow-modals; form-action 'none'; default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; media-src data: blob:; frame-ancestors 'self'"
	case kindImage:
		return "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'self'"
	}
	return "default-src 'none'; style-src 'unsafe-inline'; img-src data:; media-src 'self'; frame-ancestors 'self'"
}

// languages are the names highlight.js knows the common kinds of file by.
var languages = map[string]string{
	".go": "go", ".js": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".jsx": "javascript", ".ts": "typescript", ".tsx": "typescript",
	".py": "python", ".rb": "ruby", ".php": "php", ".java": "java", ".kt": "kotlin", ".swift": "swift", ".rs": "rust", ".c": "c", ".h": "c",
	".cpp": "cpp", ".cc": "cpp", ".hpp": "cpp", ".cs": "csharp", ".sh": "bash", ".bash": "bash", ".zsh": "bash", ".sql": "sql",
	".json": "json", ".yaml": "yaml", ".yml": "yaml", ".toml": "ini", ".ini": "ini", ".xml": "xml", ".svg": "xml", ".html": "xml", ".css": "css",
	".scss": "scss", ".less": "less", ".lua": "lua", ".r": "r", ".pl": "perl", ".diff": "diff", ".patch": "diff", ".graphql": "graphql",
}

// languageOf is what a file is highlighted as, by its name, or nothing for what has no highlighting.
func languageOf(name string) string {
	if strings.EqualFold(name, "Dockerfile") {
		return "dockerfile"
	}
	if strings.EqualFold(name, "Makefile") {
		return "makefile"
	}
	return languages[strings.ToLower(filepath.Ext(name))]
}

// linesIn is how many lines a text has: one more than its line ends, unless it ends with one.
func linesIn(text string) int {
	if text == "" {
		return 0
	}
	n := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		n++
	}
	return n
}
