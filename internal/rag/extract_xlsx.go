package rag

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
)

// maxColumns is the Excel column limit, so a crafted cell reference cannot allocate a huge row.
const maxColumns = 1 << 14

// readXLSX returns each sheet in workbook order, under its tab name.
// shortcut: cells show their stored value, so a date stays a serial number; apply styles.xml if a real corpus needs it.
func readXLSX(raw []byte) (string, error) {
	doc, err := openArchive(raw)
	if err != nil {
		return "", err
	}
	shared, err := sharedStrings(doc)
	if err != nil {
		return "", err
	}
	sheets, err := workbookSheets(doc)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, sheet := range sheets {
		data, err := doc.read(sheet.path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&out, "Onglet : %s\n\n", sheet.name)
		if err := sheetText(&out, data, shared); err != nil {
			return "", fmt.Errorf("sheet %q: %w", sheet.name, err)
		}
		out.WriteByte('\n')
	}
	return out.String(), nil
}

// sheetRef locates one worksheet of a workbook.
type sheetRef struct {
	name string
	path string
}

// workbookSheets resolves the tab names to their worksheet entries.
func workbookSheets(doc *archive) ([]sheetRef, error) {
	var book struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			ID   string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	var rels struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	for name, target := range map[string]any{"xl/workbook.xml": &book, "xl/_rels/workbook.xml.rels": &rels} {
		data, err := doc.read(name)
		if err != nil {
			return nil, err
		}
		if err := xml.Unmarshal(data, target); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	targets := make(map[string]string, len(rels.Items))
	for _, item := range rels.Items {
		targets[item.ID] = item.Target
	}
	sheets := make([]sheetRef, 0, len(book.Sheets))
	for _, sheet := range book.Sheets {
		target, ok := targets[sheet.ID]
		if !ok {
			return nil, fmt.Errorf("sheet %q has no target", sheet.Name)
		}
		entry := strings.TrimPrefix(target, "/")
		if !strings.HasPrefix(target, "/") {
			entry = path.Join("xl", target)
		}
		sheets = append(sheets, sheetRef{name: sheet.Name, path: entry})
	}
	return sheets, nil
}

// sharedStrings reads the string table; a workbook without text has none.
func sharedStrings(doc *archive) ([]string, error) {
	data, err := doc.read("xl/sharedStrings.xml")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var table []string
	var value strings.Builder
	inText := false
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return table, nil
		}
		if err != nil {
			return nil, fmt.Errorf("shared strings: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "rPh": // phonetic hint repeating the text
				if err := decoder.Skip(); err != nil {
					return nil, fmt.Errorf("shared strings: %w", err)
				}
			case "t":
				inText = true
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "t":
				inText = false
			case "si":
				table = append(table, value.String())
				value.Reset()
			}
		case xml.CharData:
			if inText {
				value.Write(element)
			}
		}
	}
}

// sheetText writes the rows of a worksheet, streaming because the row count is untrusted.
// @param out - receives the rows
// @param shared - string table of the workbook
func sheetText(out *strings.Builder, data []byte, shared []string) error {
	var cells []string
	var value strings.Builder
	column, row, previous := 0, 0, 0
	cellType, capture := "", false
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("parse xml: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "row":
				cells = cells[:0]
				row = previous + 1
				if declared, err := strconv.Atoi(xmlAttr(element, "r")); err == nil {
					row = declared
				}
			case "c":
				column = len(cells)
				if declared, ok := columnIndex(xmlAttr(element, "r")); ok {
					column = declared
				}
				if column >= maxColumns {
					return fmt.Errorf("column %d out of range", column)
				}
				cellType = xmlAttr(element, "t")
				value.Reset()
			case "rPh":
				if err := decoder.Skip(); err != nil {
					return fmt.Errorf("parse xml: %w", err)
				}
			case "v", "t":
				capture = true
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "v", "t":
				capture = false
			case "c":
				text := value.String()
				if cellType == "s" {
					index, err := strconv.Atoi(strings.TrimSpace(text))
					if err != nil || index < 0 || index >= len(shared) {
						return fmt.Errorf("invalid shared string %q", text)
					}
					text = shared[index]
				}
				for len(cells) <= column {
					cells = append(cells, "")
				}
				cells[column] = text
			case "row":
				if line := rowLine(cells); line != "" {
					if previous > 0 && row > previous+1 {
						out.WriteByte('\n')
					}
					out.WriteString(line + "\n")
					previous = row
				}
				if out.Len() > maxExtractedText {
					return errTooLarge
				}
			}
		case xml.CharData:
			if capture {
				value.Write(element)
			}
		}
	}
}

// rowLine renders one row: bare text for a single cell, "| a | b |" for a table row.
func rowLine(cells []string) string {
	last, filled := -1, 0
	for i, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			last = i
			filled++
		}
	}
	switch filled {
	case 0:
		return ""
	case 1:
		return strings.TrimSpace(cells[last])
	}
	return "| " + strings.Join(cells[:last+1], " | ") + " |"
}

// columnIndex converts a cell reference such as "C12" to its zero-based column.
func columnIndex(ref string) (int, bool) {
	column := 0
	letters := 0
	for _, char := range ref {
		if char < 'A' || char > 'Z' {
			break
		}
		// Capped so a reference of any length cannot overflow; the caller rejects the result.
		column = min(column*26+int(char-'A')+1, maxColumns+1)
		letters++
	}
	return column - 1, letters > 0
}

// xmlAttr returns an attribute value by local name, "" when absent.
func xmlAttr(element xml.StartElement, name string) string {
	for _, attr := range element.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}
