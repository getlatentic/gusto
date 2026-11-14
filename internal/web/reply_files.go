package web

import (
	"os"
	"path"
	"path/filepath"
	"slices"
)

// A reply shows the files it produced as chips under it, each the file's name with an icon for its kind, opening the file.

// sniffBytes is how much of a file is read to tell its kind, which is all a chip needs of what is in it.
const sniffBytes = 512

// chipsShown is how many chips a reply shows before the rest fold away.
const chipsShown = 6

// turnFiles is the files of one reply: the chips shown, the chips folded away, and the files that are no longer there.
type turnFiles struct {
	Shown   []fileChip
	More    []fileChip
	Removed []fileChip
}

// fileChip is one file a reply produced.
type fileChip struct {
	Path, Name string
	// Look is which icon the chip draws: image, code, doc or blank.
	Look string
	// Gone says why a file has no chip, when it has none.
	Gone string
}

// fileChips is the chips for the files a reply changed in an agent's folder, or nothing when it changed none.
func (s *Server) fileChips(instance string, changed, deleted []string) *turnFiles {
	if len(changed) == 0 {
		return nil
	}
	root := s.instances.AgentFolder(instance)
	files := &turnFiles{}
	for _, name := range changed {
		chip := fileChipOf(root, name, slices.Contains(deleted, name))
		switch {
		case chip.Gone != "":
			files.Removed = append(files.Removed, chip)
		case len(files.Shown) < chipsShown:
			files.Shown = append(files.Shown, chip)
		default:
			files.More = append(files.More, chip)
		}
	}
	return files
}

func fileChipOf(root, name string, deleted bool) fileChip {
	chip := fileChip{Path: name, Name: path.Base(name)}
	if deleted {
		chip.Gone = "was deleted"
		return chip
	}
	resolved, ok := resolveInFolder(root, filepath.Clean("/"+name))
	if !ok {
		chip.Gone = "is no longer in the folder"
		return chip
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		chip.Gone = "is no longer in the folder"
		return chip
	}
	head, _ := readSome(resolved, sniffBytes)
	chip.Look = lookOf(kindOf(chip.Name, head), chip.Name)
	return chip
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
