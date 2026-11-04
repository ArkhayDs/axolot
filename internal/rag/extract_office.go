package rag

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// maxDecompressed bounds one Office file: a zip bomb declares any size, so only bytes really read count.
const maxDecompressed = 64 << 20

var slideFile = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

// archive reads the entries of an Office zip within one shared decompression budget.
type archive struct {
	zip  *zip.Reader
	left int64
}

// openArchive parses the zip directory of an Office file.
func openArchive(raw []byte) (*archive, error) {
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	return &archive{zip: reader, left: maxDecompressed}, nil
}

// read decompresses one entry and charges it to the budget.
func (a *archive) read(name string) ([]byte, error) {
	entry, err := a.zip.Open(name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	defer entry.Close()
	data, err := io.ReadAll(io.LimitReader(entry, a.left+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if int64(len(data)) > a.left {
		return nil, errTooLarge
	}
	a.left -= int64(len(data))
	return data, nil
}

// readDOCX returns the body of a Word document.
// Headers, footers, footnotes and automatic list numbers stay out.
func readDOCX(raw []byte) (string, error) {
	doc, err := openArchive(raw)
	if err != nil {
		return "", err
	}
	data, err := doc.read("word/document.xml")
	if err != nil {
		return "", err
	}
	return markupText(data)
}

// readPPTX returns the text of each slide, speaker notes excluded.
// shortcut: slides follow their file number, which differs from the display order after a manual reorder; read presentation.xml if it matters.
func readPPTX(raw []byte) (string, error) {
	doc, err := openArchive(raw)
	if err != nil {
		return "", err
	}
	numbers := []int{}
	for _, file := range doc.zip.File {
		if match := slideFile.FindStringSubmatch(file.Name); match != nil {
			number, err := strconv.Atoi(match[1])
			if err != nil {
				return "", fmt.Errorf("%s: %w", file.Name, err)
			}
			numbers = append(numbers, number)
		}
	}
	sort.Ints(numbers)
	var out strings.Builder
	for _, number := range numbers {
		data, err := doc.read(fmt.Sprintf("ppt/slides/slide%d.xml", number))
		if err != nil {
			return "", err
		}
		text, err := markupText(data)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&out, "Diapositive %d\n\n%s\n", number, text)
	}
	return out.String(), nil
}

// markupText renders WordprocessingML or DrawingML, which share these element names.
// Table rows keep the markdown shape "| a | b |", so one row stays one searchable line.
func markupText(data []byte) (string, error) {
	var out strings.Builder
	decoder := xml.NewDecoder(bytes.NewReader(data))
	inText, inCell, cellStarted := false, false, false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return out.String(), nil
		}
		if err != nil {
			return "", fmt.Errorf("parse xml: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			// pPr declares tab stops with the same element name; Fallback repeats the Choice content.
			case "pPr", "Fallback":
				if err := decoder.Skip(); err != nil {
					return "", fmt.Errorf("parse xml: %w", err)
				}
			case "t":
				inText = true
			case "tab":
				out.WriteByte('\t')
			case "br", "cr":
				out.WriteByte('\n')
			case "tr":
				out.WriteByte('|')
			case "tc":
				out.WriteByte(' ')
				inCell, cellStarted = true, false
			case "p":
				if inCell && cellStarted {
					out.WriteByte(' ')
				}
				cellStarted = true
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "t":
				inText = false
			case "p":
				if !inCell {
					out.WriteString("\n\n")
				}
			case "tc":
				out.WriteString(" |")
				inCell = false
			case "tr":
				out.WriteByte('\n')
			case "tbl":
				out.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				out.Write(element)
			}
		}
	}
}
