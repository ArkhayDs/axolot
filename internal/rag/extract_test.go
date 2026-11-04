package rag

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// buildZip assembles an Office-like archive in memory from name, content pairs.
func buildZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// buildPDF writes a minimal PDF with one content stream per page, in WinAnsi encoding.
func buildPDF(pages ...string) []byte {
	kids := []string{}
	for i := range pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 4+2*i))
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	for i, page := range pages {
		stream := "BT /F1 12 Tf 50 700 Td (" + page + ") Tj ET\n"
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /Contents %d 0 R /Resources << /Font << /F1 3 0 R >> >> >>", 5+2*i),
			fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream))
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return out.Bytes()
}

// wantText extracts a document and checks each expected line is present.
func wantText(t *testing.T, name string, raw []byte, want ...string) string {
	t.Helper()
	text, err := Extract(name, raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range want {
		if !strings.Contains(text, line) {
			t.Errorf("missing %q in:\n%s", line, text)
		}
	}
	return text
}

// Plain formats stay byte for byte, the chunk offsets of existing corpora depend on it.
func TestExtractKeepsPlainTextUntouched(t *testing.T) {
	const content = "  # Titre\n\n**gras** | é \n"
	for _, name := range []string{"a.md", "a.txt", "A.MD"} {
		text, err := Extract(name, []byte(content))
		if err != nil || text != content {
			t.Fatalf("%s: %q, %v", name, text, err)
		}
	}
	if _, err := Extract("a.md", []byte{0xff}); err == nil || !strings.Contains(err.Error(), "a.md") {
		t.Fatalf("invalid UTF-8 not refused by name: %v", err)
	}
}

// Runs join into one paragraph; deleted text, tab stops and fallback copies stay out.
func TestExtractDOCX(t *testing.T) {
	const doc = `<w:document xmlns:w="w" xmlns:mc="mc"><w:body>
		<w:p><w:pPr><w:tabs><w:tab w:val="left"/></w:tabs></w:pPr><w:r><w:t>Une phrase </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>accentuée</w:t></w:r><w:r><w:t xml:space="preserve"> en trois &amp; morceaux.</w:t></w:r></w:p>
		<w:p><w:r><w:t>avant</w:t><w:tab/><w:t>après</w:t><w:br/><w:t>ligne</w:t></w:r></w:p>
		<w:p><w:r><w:delText>supprimé</w:delText></w:r><mc:AlternateContent><mc:Choice><w:t>choix</w:t></mc:Choice><mc:Fallback><w:t>doublon</w:t></mc:Fallback></mc:AlternateContent></w:p>
		<w:tbl><w:tr><w:tc><w:p><w:r><w:t>Jalon</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>J+45</w:t></w:r></w:p><w:p><w:r><w:t>RH</w:t></w:r></w:p></w:tc></w:tr></w:tbl>
		<w:p><w:r><w:t>fin</w:t></w:r></w:p>
	</w:body></w:document>`
	text := wantText(t, "a.docx", buildZip(t, map[string]string{"word/document.xml": doc}),
		"Une phrase accentuée en trois & morceaux.", "avant\taprès\nligne", "choix", "| Jalon | J+45 RH |\n", "fin")
	for _, absent := range []string{"supprimé", "doublon"} {
		if strings.Contains(text, absent) {
			t.Errorf("%q leaked in:\n%s", absent, text)
		}
	}
	if strings.HasPrefix(text, "\t") {
		t.Errorf("tab stop declaration rendered as text: %q", text)
	}
}

// Slide 10 comes after slide 2, and speaker notes stay out.
func TestExtractPPTXFollowsSlideNumbers(t *testing.T) {
	slide := func(text string) string {
		return `<p:sld xmlns:p="p" xmlns:a="a"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:rPr b="1"/><a:t>` + text + `</a:t></a:r><a:r><a:t> suite</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
	}
	text := wantText(t, "a.pptx", buildZip(t, map[string]string{
		"ppt/slides/slide2.xml":  slide("Deuxième"),
		"ppt/slides/slide10.xml": slide("Dixième"),
		"ppt/notesSlides/n.xml":  slide("Note"),
	}), "Diapositive 2\n\nDeuxième suite", "Diapositive 10\n\nDixième suite")
	if strings.Index(text, "Deuxième") > strings.Index(text, "Dixième") {
		t.Errorf("slide order: %s", text)
	}
	if strings.Contains(text, "Note") {
		t.Errorf("notes leaked: %s", text)
	}
}

// buildXLSX assembles a workbook with one sheet per tab, tabs given in display order.
func buildXLSX(t *testing.T, shared string, tabs [][2]string) []byte {
	t.Helper()
	files := map[string]string{"xl/sharedStrings.xml": shared}
	sheets, rels := "", ""
	for i, tab := range tabs {
		sheets += fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, tab[0], i+1, i+1)
		rels += fmt.Sprintf(`<Relationship Id="rId%d" Target="worksheets/s%d.xml"/>`, i+1, i+1)
		files[fmt.Sprintf("xl/worksheets/s%d.xml", i+1)] = tab[1]
	}
	files["xl/workbook.xml"] = `<workbook xmlns:r="r"><sheets>` + sheets + `</sheets></workbook>`
	files["xl/_rels/workbook.xml.rels"] = `<Relationships>` + rels + `</Relationships>`
	return buildZip(t, files)
}

// Shared and inline strings resolve, and table rows keep the "| a | b |" shape that quotes rely on.
func TestExtractXLSX(t *testing.T) {
	const shared = `<sst><si><t>Zone</t></si><si><r><t>Tarif </t></r><r><t>été</t></r><rPh><t>phonétique</t></rPh></si><si><t>Une phrase seule.</t></si></sst>`
	const grid = `<worksheet><sheetData>
		<row r="1"><c r="A1" t="s"><v>2</v></c></row>
		<row r="3"><c r="A3" t="s"><v>0</v></c><c r="B3" t="s"><v>1</v></c></row>
		<row r="4"><c r="A4" t="inlineStr"><is><t>Brico C</t></is></c><c r="C4"><f>1+1</f><v>35.5</v></c></row>
		<row r="5"/>
	</sheetData></worksheet>`
	const other = `<worksheet><sheetData><row><c t="str"><v>a</v></c><c t="b"><v>1</v></c></row></sheetData></worksheet>`
	text := wantText(t, "a.xlsx", buildXLSX(t, shared, [][2]string{{"Grille été", grid}, {"Autre", other}}),
		"Onglet : Grille été\n\nUne phrase seule.\n\n| Zone | Tarif été |\n| Brico C |  | 35.5 |\n",
		"Onglet : Autre\n\n| a | 1 |")
	if strings.Contains(text, "phonétique") {
		t.Errorf("phonetic hint leaked: %s", text)
	}
	if strings.Index(text, "Grille") > strings.Index(text, "Autre") {
		t.Errorf("tab order: %s", text)
	}
}

// Out-of-range references must fail by name rather than index out of bounds or allocate.
func TestExtractXLSXRefusesCraftedCells(t *testing.T) {
	sheet := func(cell string) []byte {
		return buildXLSX(t, `<sst><si><t>x</t></si></sst>`, [][2]string{{"T", `<worksheet><sheetData><row r="1">` + cell + `</row></sheetData></worksheet>`}})
	}
	for name, cell := range map[string]string{
		"shared index out of range": `<c r="A1" t="s"><v>9</v></c>`,
		"shared index negative":     `<c r="A1" t="s"><v>-1</v></c>`,
		"column out of range":       `<c r="ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ1"><v>1</v></c>`,
	} {
		if _, err := Extract("bad.xlsx", sheet(cell)); err == nil || !strings.Contains(err.Error(), "bad.xlsx") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Quoted-printable soft breaks must vanish, or a cited sentence would no longer match.
func TestExtractEML(t *testing.T) {
	const headers = "From: =?utf-8?q?Direction_des_ressources_humaines?= <drh@helianthe.example>\r\nTo: tous@helianthe.example\r\nDate: Thu, 12 Feb 2026 10:30:00 +0100\r\nSubject: =?utf-8?q?Avenant_r=C3=A9gime?=\r\n"
	flat := headers + "Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nUne phrase accentu=C3=A9e coup=\r\n=C3=A9e par le format.\r\nSuite.\r\n"
	wantText(t, "a.eml", []byte(flat),
		"De : Direction des ressources humaines <drh@helianthe.example>\n", "À : tous@helianthe.example\n",
		"Date : Thu, 12 Feb 2026 10:30:00 +0100\n", "Objet : Avenant régime\n",
		"Une phrase accentuée coupée par le format.\nSuite.")

	const multi = "Content-Type: multipart/mixed; boundary=out\r\n\r\n--out\r\nContent-Type: multipart/alternative; boundary=in\r\n\r\n--in\r\nContent-Type: text/html\r\n\r\n<b>html</b>\r\n--in\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\nQ29ycHMgZW4gYmFzZTY0IMOpdMOp\r\n--in--\r\n--out\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=x.txt\r\n\r\npièce jointe\r\n--out--\r\n"
	text := wantText(t, "b.eml", []byte(headers+multi), "Corps en base64 été")
	if strings.Contains(text, "html") || strings.Contains(text, "pièce jointe") {
		t.Errorf("wrong part selected: %s", text)
	}
}

// What the extractor cannot read faithfully is refused, not half-indexed.
func TestExtractEMLRefusals(t *testing.T) {
	nested := "Content-Type: multipart/mixed; boundary=b\r\n\r\n"
	for range maxMailDepth + 2 {
		nested = "Content-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\n" + nested
	}
	for name, raw := range map[string]string{
		"html only":     "Subject: x\r\nContent-Type: text/html\r\n\r\n<b>x</b>",
		"other charset": "Subject: x\r\nContent-Type: text/plain; charset=iso-8859-1\r\n\r\nx",
		"bad utf-8":     "Subject: x\r\n\r\n\xff\xfe",
		"not a message": "\xff\xfe no header",
		"too deep":      nested,
	} {
		if _, err := Extract("bad.eml", []byte(raw)); err == nil || !strings.Contains(err.Error(), "bad.eml") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// WinAnsi accents and PDF string escapes decode, across pages in order.
func TestExtractPDFKeepsAccentsAndPages(t *testing.T) {
	first := "Proc\xe9dure \\(accentu\xe9e\\) \xe0 l'\xe9t\xe9"
	text := wantText(t, "a.pdf", buildPDF(first, "Seconde page"), "Procédure (accentuée) à l'été", "Seconde page")
	if strings.Index(text, "Procédure") > strings.Index(text, "Seconde") {
		t.Errorf("page order: %s", text)
	}
}

// Every failure path names the file, including a document that parses but holds no text.
func TestExtractRefusesUnreadableDocuments(t *testing.T) {
	noText := buildZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="w"><w:body><w:p/></w:body></w:document>`})
	valid := buildPDF("Bonjour")
	truncated := valid[:len(valid)/2]
	corrupt := bytes.ReplaceAll(buildPDF("Bonjour"), []byte("Tj"), []byte("(("))
	cases := map[string][]byte{
		"garbage.docx":   []byte("pas un zip"),
		"garbage.pptx":   []byte("pas un zip"),
		"garbage.xlsx":   []byte("pas un zip"),
		"garbage.pdf":    []byte("pas un pdf"),
		"empty.pdf":      {},
		"truncated.pdf":  truncated,
		"corrupt.pdf":    corrupt,
		"notext.docx":    noText,
		"nobody.docx":    buildZip(t, map[string]string{"other.xml": "<a/>"}),
		"badxml.docx":    buildZip(t, map[string]string{"word/document.xml": "<w:t>"}),
		"noslide.pptx":   buildZip(t, map[string]string{"ppt/presentation.xml": "<a/>"}),
		"nobook.xlsx":    buildZip(t, map[string]string{"xl/sharedStrings.xml": "<sst/>"}),
		"blank.pdf":      buildPDF(),
		"unknown.rtf":    []byte("x"),
		"nosuffixformat": []byte("x"),
	}
	for name, raw := range cases {
		if _, err := Extract(name, raw); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: want an error naming the file, got %v", name, err)
		}
	}
}

// The PDF library panics on a malformed dictionary: this input must come back as an error naming the file.
func TestExtractSurvivesRealParserPanic(t *testing.T) {
	raw := bytes.Replace(buildPDF("Bonjour"), []byte("<< /Type /Catalog"), []byte("<<0/Type /Catalog"), 1)
	_, err := Extract("panic.pdf", raw)
	if err == nil || !strings.Contains(err.Error(), "panic.pdf") || !strings.Contains(err.Error(), "parser panic") {
		t.Fatalf("got %v", err)
	}
}

// A bomb declares nothing useful: only the bytes really inflated may count, across all entries.
func TestExtractBoundsDecompressedSize(t *testing.T) {
	padding := strings.Repeat(" ", maxDecompressed/2+1)
	inOneEntry := buildZip(t, map[string]string{"word/document.xml": padding + padding})
	if _, err := Extract("bomb.docx", inOneEntry); !errors.Is(err, errTooLarge) || !strings.Contains(err.Error(), "bomb.docx") {
		t.Errorf("single entry: %v", err)
	}
	slide := "<a>" + padding + "</a>"
	acrossEntries := buildZip(t, map[string]string{"ppt/slides/slide1.xml": slide, "ppt/slides/slide2.xml": slide})
	if _, err := Extract("bomb.pptx", acrossEntries); !errors.Is(err, errTooLarge) {
		t.Errorf("across entries: %v", err)
	}
}

// A library panic in the goroutine would kill the server, and a hang would block the request.
func TestGuardedRecoversPanicAndHang(t *testing.T) {
	if _, err := guarded(time.Second, func() (string, error) { panic("boom") }); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("panic: %v", err)
	}
	release := make(chan struct{})
	defer close(release)
	_, err := guarded(10*time.Millisecond, func() (string, error) {
		<-release
		return "", nil
	})
	if err == nil {
		t.Error("hang: want an error")
	}
	if text, err := guarded(time.Second, func() (string, error) { return "ok", nil }); text != "ok" || err != nil {
		t.Errorf("success: %q, %v", text, err)
	}
}

// Listing follows the single extension list.
func TestSupportedAndFilesShareOneList(t *testing.T) {
	corpus := fstest.MapFS{}
	for _, ext := range Extensions() {
		corpus["doc"+ext] = &fstest.MapFile{}
	}
	corpus["image.png"] = &fstest.MapFile{}
	corpus["archive.docx.exe"] = &fstest.MapFile{}
	names, err := Files(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != len(Extensions()) {
		t.Fatalf("names: %v", names)
	}
	if !Supported("RAPPORT.PDF") || Supported("x.docm") {
		t.Error("Supported ignores case only")
	}
}

// chunkStore keeps the chunks that would be persisted.
type chunkStore struct {
	fakeStore
	chunks []Chunk
}

// ReplaceChunks records the chunks instead of writing them.
func (s *chunkStore) ReplaceChunks(_ context.Context, _, _ string, chunks []Chunk, _ [][]float32) error {
	s.chunks = chunks
	return nil
}

// Ingest must index extracted text, and a broken document must abort before touching the index.
func TestIngestUsesExtraction(t *testing.T) {
	good := buildZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="w"><w:p><w:r><w:t>Texte du document Word.</w:t></w:r></w:p></w:document>`})
	store := &chunkStore{}
	total, err := Ingest(t.Context(), store, fakeEmbedder{}, "m", "c", fstest.MapFS{"a.docx": {Data: good}}, 200, 0, func(int, int) {})
	if err != nil || total != 1 {
		t.Fatalf("total=%d err=%v", total, err)
	}
	if store.chunks[0].File != "a.docx" || store.chunks[0].Content != "Texte du document Word." {
		t.Fatalf("chunk: %+v", store.chunks[0])
	}
	store = &chunkStore{}
	_, err = Ingest(t.Context(), store, fakeEmbedder{}, "m", "c", fstest.MapFS{"a.docx": {Data: good}, "b.pdf": {Data: []byte("cassé")}}, 200, 0, func(int, int) {})
	if err == nil || !strings.Contains(err.Error(), "b.pdf") || store.chunks != nil {
		t.Fatalf("err=%v chunks=%v", err, store.chunks)
	}
}
