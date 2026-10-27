package web

import (
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// A room shows the files a reply produced as cards under it: the file's name, size and a download, over a preview of its top. The
// preview is read from the room's folder as it is now, the same file the card opens.

// previewBytes is the most of a file read to preview it, since a file in a folder can be any size.
const previewBytes = 16 << 10

// How many lines of a text or Markdown file a card shows, and how many cards a reply shows before the rest fold away.
const (
	previewLines  = 12
	markdownLines = 40
	cardsShown    = 2
)

// turnFiles is the files of one reply: the cards shown, the cards folded away, and the files that are no longer there.
type turnFiles struct {
	Cards   []fileCard
	More    []fileCard
	Removed []fileCard
}

// fileCard is one file a reply produced, as its card draws it.
type fileCard struct {
	Path, Name, Size string
	Kind             fileKind
	// Look is which icon the card draws: image, code, doc or blank.
	Look          string
	Raw, Download string
	// Gone says why a file has no card, when it has none.
	Gone string
	// Text is the first lines of a text file, and Page the top of a Markdown file drawn.
	Text string
	Page template.HTML
}

// fileCards is the cards for the files a reply changed in a room's folder, or nothing when it changed none.
func (s *Server) fileCards(room string, changed, deleted []string) *turnFiles {
	if len(changed) == 0 {
		return nil
	}
	root := s.instances.Workspace(room)
	files := &turnFiles{}
	for _, name := range changed {
		card := s.fileCard(room, root, name, slices.Contains(deleted, name))
		switch {
		case card.Gone != "":
			files.Removed = append(files.Removed, card)
		case len(files.Cards) < cardsShown:
			files.Cards = append(files.Cards, card)
		default:
			files.More = append(files.More, card)
		}
	}
	return files
}

func (s *Server) fileCard(room, root, name string, deleted bool) fileCard {
	raw := "/instances/" + room + "/raw?path=" + url.QueryEscape(name)
	card := fileCard{Path: name, Name: path.Base(name), Raw: raw, Download: raw + "&download=1"}
	if deleted {
		card.Gone = "was deleted"
		return card
	}
	resolved, ok := resolveInFolder(root, filepath.Clean("/"+name))
	if !ok {
		card.Gone = "is no longer in the folder"
		return card
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		card.Gone = "is no longer in the folder"
		return card
	}
	head, _ := readSome(resolved, previewBytes)
	card.Kind, card.Size = kindOf(card.Name, head), sizeWords(info.Size())
	card.Look = lookOf(card.Kind, card.Name)
	s.preview(&card, room, head, info.Size() > previewBytes)
	return card
}

// preview reads what the card shows of a file of its kind. A web page is not read at all: the card frames it from the file's own
// address, sealed, so nothing an agent wrote runs in the room's page.
func (s *Server) preview(card *fileCard, room string, head []byte, cut bool) {
	switch card.Kind {
	case kindMarkdown:
		dir := strings.TrimPrefix(path.Dir("/"+card.Path), "/")
		card.Page = s.replyIn(topOf(head, markdownLines, cut), room, dir, "/instances/"+room+"/file?path="+url.QueryEscape(card.Path))
	case kindText, kindTable:
		card.Text = topOf(head, previewLines, cut)
	}
}

// topOf is the first lines of what was read of a file. A file longer than what was read loses its last, partial line, which could
// end in the middle of a character or a word.
func topOf(head []byte, lines int, cut bool) string {
	text := string(head)
	if cut {
		if end := strings.LastIndexByte(text, '\n'); end >= 0 {
			text = text[:end]
		}
	}
	kept := strings.SplitN(text, "\n", lines+1)
	kept = kept[:min(len(kept), lines)]
	return strings.ToValidUTF8(strings.TrimRight(strings.Join(kept, "\n"), "\n"), "")
}

func lookOf(kind fileKind, name string) string {
	switch {
	case kind == kindImage:
		return "image"
	case kind == kindHTML || languageOf(name) != "":
		return "code"
	case kind == kindBinary || kind == kindAudio || kind == kindVideo:
		return "blank"
	}
	return "doc"
}

// sizeWords is a file's size as a card says it, in whole kilobytes under a megabyte.
func sizeWords(size int64) string {
	switch {
	case size >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("%d KB", (size+1<<9)>>10)
	}
	return fmt.Sprintf("%d bytes", size)
}
