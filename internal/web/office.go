package web

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// What a Word, PowerPoint or Excel file says, read from the text in the file's own XML. It is a reading of the words, not a drawing of the
// document: the page says so, and offers the file itself.

const (
	// mostPartRead is the most that is read of one part of such a file, which is a zip and can be made to be a great deal larger than it is.
	mostPartRead = 8 << 20
	mostParts    = 600
	mostRows     = 200
	mostColumns  = 26
)

// officePart is one stretch of what a file says: a slide, a sheet, or the body of a document.
type officePart struct {
	Title string
	Lines []officeLine
	Rows  [][]string
}

// officeLine is one paragraph, with the style it was written in when that says it is a heading.
type officeLine struct {
	Text    string
	Heading int
}

// readOffice reads a .docx, .pptx or .xlsx. It says nothing for a file it cannot read as one.
func readOffice(path, ext string) ([]officePart, bool) {
	archive, err := zip.OpenReader(path)
	if err != nil || len(archive.File) > mostParts {
		return nil, false
	}
	defer archive.Close()
	parts := map[string]*zip.File{}
	for _, file := range archive.File {
		parts[file.Name] = file
	}
	switch ext {
	case ".docx":
		return docxParts(parts)
	case ".pptx":
		return pptxParts(parts)
	case ".xlsx":
		return xlsxParts(parts)
	}
	return nil, false
}

func partReader(file *zip.File) io.ReadCloser {
	opened, err := file.Open()
	if err != nil {
		return nil
	}
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(opened, mostPartRead), opened}
}

var headingStyle = regexp.MustCompile(`(?i)^(title|heading\s*([1-6]))$`)

// docxParts is the paragraphs of a document, in order, with the ones styled as headings marked.
func docxParts(parts map[string]*zip.File) ([]officePart, bool) {
	file := parts["word/document.xml"]
	if file == nil {
		return nil, false
	}
	reader := partReader(file)
	if reader == nil {
		return nil, false
	}
	defer reader.Close()
	var lines []officeLine
	var text strings.Builder
	heading, inText := 0, false
	decoder := xml.NewDecoder(reader)
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "p":
				text.Reset()
				heading = 0
			case "pStyle":
				for _, attribute := range element.Attr {
					if match := headingStyle.FindStringSubmatch(attribute.Value); match != nil {
						heading = 1
						if level, err := strconv.Atoi(match[2]); err == nil {
							heading = level
						}
					}
				}
			case "t":
				inText = true
			case "tab":
				text.WriteString("\t")
			case "br":
				text.WriteString("\n")
			}
		case xml.CharData:
			if inText {
				text.Write(element)
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "t":
				inText = false
			case "p":
				if line := strings.TrimSpace(text.String()); line != "" {
					lines = append(lines, officeLine{Text: line, Heading: heading})
				}
			}
		}
	}
	return []officePart{{Lines: lines}}, len(lines) > 0
}

var slideName = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

// pptxParts is the text of each slide, the first line of it as the slide's title.
func pptxParts(parts map[string]*zip.File) ([]officePart, bool) {
	var numbered []int
	for name := range parts {
		if match := slideName.FindStringSubmatch(name); match != nil {
			n, _ := strconv.Atoi(match[1])
			numbered = append(numbered, n)
		}
	}
	sort.Ints(numbered)
	var slides []officePart
	for _, n := range numbered {
		reader := partReader(parts["ppt/slides/slide"+strconv.Itoa(n)+".xml"])
		if reader == nil {
			continue
		}
		slide := officePart{Title: "Slide " + strconv.Itoa(n)}
		var text strings.Builder
		inText := false
		decoder := xml.NewDecoder(reader)
		for {
			token, err := decoder.Token()
			if err != nil {
				break
			}
			switch element := token.(type) {
			case xml.StartElement:
				if element.Name.Local == "p" {
					text.Reset()
				}
				inText = element.Name.Local == "t"
			case xml.CharData:
				if inText {
					text.Write(element)
				}
			case xml.EndElement:
				if element.Name.Local == "t" {
					inText = false
				}
				if element.Name.Local == "p" {
					if line := strings.TrimSpace(text.String()); line != "" {
						slide.Lines = append(slide.Lines, officeLine{Text: line})
					}
				}
			}
		}
		reader.Close()
		slides = append(slides, slide)
	}
	return slides, len(slides) > 0
}

// xlsxParts is the cells of each sheet as rows, the first rows and columns of it.
func xlsxParts(parts map[string]*zip.File) ([]officePart, bool) {
	shared := sharedStrings(parts["xl/sharedStrings.xml"])
	var names []string
	for name := range parts {
		if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml") {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(a, b int) bool { return sheetNumber(names[a]) < sheetNumber(names[b]) })
	var sheets []officePart
	for _, name := range names {
		reader := partReader(parts[name])
		if reader == nil {
			continue
		}
		sheet := officePart{Title: "Sheet " + strconv.Itoa(sheetNumber(name))}
		sheet.Rows = sheetRows(reader, shared)
		reader.Close()
		sheets = append(sheets, sheet)
	}
	return sheets, len(sheets) > 0
}

func sheetNumber(name string) int {
	digits := strings.TrimSuffix(strings.TrimPrefix(name, "xl/worksheets/sheet"), ".xml")
	n, _ := strconv.Atoi(digits)
	return n
}

func sharedStrings(file *zip.File) []string {
	if file == nil {
		return nil
	}
	reader := partReader(file)
	if reader == nil {
		return nil
	}
	defer reader.Close()
	var all []string
	var text strings.Builder
	inText := false
	decoder := xml.NewDecoder(reader)
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Local == "si" {
				text.Reset()
			}
			inText = element.Name.Local == "t"
		case xml.CharData:
			if inText {
				text.Write(element)
			}
		case xml.EndElement:
			if element.Name.Local == "t" {
				inText = false
			}
			if element.Name.Local == "si" {
				all = append(all, text.String())
			}
		}
	}
	return all
}

var cellColumn = regexp.MustCompile(`^[A-Z]+`)

func columnNumber(reference string) int {
	letters := cellColumn.FindString(reference)
	n := 0
	for _, letter := range letters {
		n = n*26 + int(letter-'A') + 1
	}
	return n - 1
}

func sheetRows(reader io.Reader, shared []string) [][]string {
	var rows [][]string
	var row []string
	column, kind := 0, ""
	inValue, inline := false, false
	decoder := xml.NewDecoder(reader)
	for len(rows) < mostRows {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "row":
				row = nil
			case "c":
				kind, column = "", len(row)
				for _, attribute := range element.Attr {
					switch attribute.Name.Local {
					case "t":
						kind = attribute.Value
					case "r":
						column = columnNumber(attribute.Value)
					}
				}
			case "v":
				inValue = true
			case "t":
				inline = kind == "inlineStr"
			}
		case xml.CharData:
			if (inValue || inline) && column < mostColumns {
				for len(row) <= column {
					row = append(row, "")
				}
				value := string(element)
				if kind == "s" {
					if index, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && index < len(shared) {
						value = shared[index]
					}
				}
				row[column] += value
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "v":
				inValue = false
			case "t":
				inline = false
			case "row":
				rows = append(rows, row)
			}
		}
	}
	return rows
}
