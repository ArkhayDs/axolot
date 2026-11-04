package rag

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

const (
	// maxExtractedText bounds the text kept from one document, whatever its size on disk.
	maxExtractedText = 32 << 20
	maxPDFPages      = 5000
	maxMailDepth     = 8
	pdfTimeout       = 20 * time.Second
)

var (
	errTooLarge = errors.New("document too large once decompressed")
	errNoText   = errors.New("no text found")
)

// formats is the only list of accepted extensions: listing, upload and reading all derive from it.
// A nil read means plain text, kept byte for byte.
var formats = []struct {
	ext  string
	read func([]byte) (string, error)
}{
	{ext: ".md"},
	{ext: ".txt"},
	{ext: ".docx", read: readDOCX},
	{ext: ".pptx", read: readPPTX},
	{ext: ".xlsx", read: readXLSX},
	{ext: ".eml", read: readEML},
	{ext: ".pdf", read: readPDF},
}

// Extensions returns the accepted document extensions, for display and client filters.
func Extensions() []string {
	exts := make([]string, 0, len(formats))
	for _, format := range formats {
		exts = append(exts, format.ext)
	}
	return exts
}

// Supported reports whether a file name carries an accepted extension.
func Supported(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	for _, format := range formats {
		if format.ext == ext {
			return true
		}
	}
	return false
}

// Extract returns the text of a document, chosen by extension.
// Content is untrusted: every error names the file and a document without text is refused.
// @param name - file name, reported in errors
// @param raw - document bytes
func Extract(name string, raw []byte) (string, error) {
	ext := strings.ToLower(path.Ext(name))
	for _, format := range formats {
		if format.ext != ext {
			continue
		}
		if format.read == nil {
			if !utf8.Valid(raw) {
				return "", fmt.Errorf("%s is not UTF-8", name)
			}
			return string(raw), nil
		}
		text, err := format.read(raw)
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		// PostgreSQL refuses NUL in text, and a PDF font can decode to anything.
		text = strings.TrimSpace(strings.ReplaceAll(strings.ToValidUTF8(text, ""), "\x00", ""))
		if text == "" {
			return "", fmt.Errorf("%s: %w", name, errNoText)
		}
		return text, nil
	}
	return "", fmt.Errorf("%s: unsupported format", name)
}

// -----------------------------------------------------------------------------
// E-mail
// -----------------------------------------------------------------------------

// readEML returns the main headers and the text body of a message.
// shortcut: HTML-only and non UTF-8 bodies are refused, add a converter if a real corpus needs them.
func readEML(raw []byte) (string, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("read message: %w", err)
	}
	body, err := mailBody(textproto.MIMEHeader(msg.Header), msg.Body, 0)
	if err != nil {
		return "", err
	}
	if body == "" {
		return "", errors.New("no text/plain body")
	}
	var out strings.Builder
	for _, field := range [][2]string{{"De", "From"}, {"À", "To"}, {"Date", "Date"}, {"Objet", "Subject"}} {
		if value := decodeHeader(msg.Header.Get(field[1])); value != "" {
			fmt.Fprintf(&out, "%s : %s\n", field[0], value)
		}
	}
	out.WriteString("\n" + body)
	return out.String(), nil
}

// decodeHeader resolves RFC 2047 encoded words, keeping the raw value for an unknown charset.
func decodeHeader(value string) string {
	decoded, err := new(mime.WordDecoder).DecodeHeader(value)
	if err != nil {
		return value
	}
	return decoded
}

// mailBody returns the first inline text/plain part, or "" when the entity holds none.
// @param depth - multipart nesting so far, bounded against crafted messages
func mailBody(header textproto.MIMEHeader, body io.Reader, depth int) (string, error) {
	if depth > maxMailDepth {
		return "", errors.New("multipart nesting too deep")
	}
	mediaType, params := "text/plain", map[string]string{}
	if contentType := header.Get("Content-Type"); contentType != "" {
		var err error
		if mediaType, params, err = mime.ParseMediaType(contentType); err != nil {
			return "", fmt.Errorf("content type: %w", err)
		}
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		parts := multipart.NewReader(body, params["boundary"])
		for {
			part, err := parts.NextRawPart()
			if errors.Is(err, io.EOF) {
				return "", nil
			}
			if err != nil {
				return "", fmt.Errorf("multipart: %w", err)
			}
			text, err := mailBody(part.Header, part, depth+1)
			if err != nil || text != "" {
				return text, err
			}
		}
	}
	if mediaType != "text/plain" || strings.HasPrefix(strings.ToLower(header.Get("Content-Disposition")), "attachment") {
		return "", nil
	}
	if charset := strings.ToLower(params["charset"]); charset != "" && charset != "utf-8" && charset != "us-ascii" {
		return "", fmt.Errorf("unsupported charset %q", charset)
	}
	decoded, err := io.ReadAll(transferDecoder(header.Get("Content-Transfer-Encoding"), body))
	if err != nil {
		return "", fmt.Errorf("decode body: %w", err)
	}
	if !utf8.Valid(decoded) {
		return "", errors.New("body is not UTF-8")
	}
	return strings.ReplaceAll(string(decoded), "\r\n", "\n"), nil
}

// transferDecoder undoes a Content-Transfer-Encoding; other values are already plain.
func transferDecoder(encoding string, body io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		return quotedprintable.NewReader(body)
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, body)
	}
	return body
}

// -----------------------------------------------------------------------------
// PDF
// -----------------------------------------------------------------------------

// readPDF extracts text page by page, guarded against the parser on untrusted input.
func readPDF(raw []byte) (string, error) {
	return guarded(pdfTimeout, func() (string, error) { return parsePDF(raw) })
}

// guarded runs a parser in its own goroutine, turning a panic or a hang into an error.
// shortcut: a hung goroutine cannot be killed and spins until restart, move parsing to a subprocess if this happens.
// @param timeout - longest wait for parse
func guarded(timeout time.Duration, parse func() (string, error)) (string, error) {
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{err: fmt.Errorf("parser panic: %v", r)}
			}
		}()
		text, err := parse()
		done <- result{text: text, err: err}
	}()
	select {
	case res := <-done:
		return res.text, res.err
	case <-time.After(timeout):
		return "", fmt.Errorf("parser gave no result after %s", timeout)
	}
}

// parsePDF concatenates the plain text of each page.
func parsePDF(raw []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}
	pages := reader.NumPage()
	if pages > maxPDFPages {
		return "", errTooLarge
	}
	var out strings.Builder
	for number := 1; number <= pages; number++ {
		page := reader.Page(number)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("page %d: %w", number, err)
		}
		out.WriteString(text + "\n\n")
		if out.Len() > maxExtractedText {
			return "", errTooLarge
		}
	}
	return out.String(), nil
}
