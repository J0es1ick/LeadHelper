package filepreview

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func archive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for name, text := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestPassiveTextAndTypeDetection(t *testing.T) {
	markup := `<script>alert(document.cookie)</script>`
	for _, name := range []string{"note.html", "drawing.svg", "note.txt"} {
		p := Build(name, "text/html", []byte(markup))
		if p.Kind != "text" || p.Text != markup || InlineType([]byte(markup)) != "" {
			t.Fatalf("active preview for %s: %+v", name, p)
		}
	}
	if p := Build("fake.pdf", "application/pdf", []byte(markup)); p.Kind != "unsupported" {
		t.Fatal("sender MIME trusted")
	}
	if p := Build("binary.txt", "text/plain", []byte{0, 1, 2}); p.Kind != "unsupported" {
		t.Fatal("binary rendered as text")
	}
	if p := Build("real.pdf", "application/octet-stream", []byte("%PDF-1.4\n")); p.Kind != "pdf" {
		t.Fatal("PDF signature ignored")
	}
	if p := Build("image.dat", "application/octet-stream", []byte("\x89PNG\r\n\x1a\n")); p.Kind != "image" {
		t.Fatal("image signature ignored")
	}
	encoded, _ := charmap.Windows1251.NewEncoder().Bytes([]byte("Привет"))
	if p := Build("note.txt", "text/plain; charset=windows-1251", encoded); p.Text != "Привет" {
		t.Fatal("Cyrillic lost")
	}
	if p := Build("note.txt", "text/plain", []byte{0xff, 0xfe, 0x1f, 0x04}); p.Text != "П" {
		t.Fatal("UTF16 lost")
	}
	if p := Build("huge.txt", "text/plain", bytes.Repeat([]byte("a"), maxText+1)); !p.Truncated || len(p.Text) != maxText {
		t.Fatal("text limit ignored")
	}
}

func TestCSVQuotedFieldsAndLimits(t *testing.T) {
	p := Build("report.csv", "text/csv", []byte("Имя;Комментарий\nЮрий;\"строка; с разделителем\"\n"))
	if p.Kind != "table" || p.Sheets[0].Rows[1][1] != "строка; с разделителем" {
		t.Fatalf("CSV: %+v", p)
	}
	p = Build("report.csv", "text/csv", []byte(strings.Repeat("one,two\n", maxRows+1)))
	if !p.Truncated || len(p.Sheets[0].Rows) != maxRows {
		t.Fatal("CSV row limit ignored")
	}
}

func TestOfficeTextAndCorruptFiles(t *testing.T) {
	p := Build("document.docx", "", archive(t, map[string]string{"word/document.xml": `<w:document xmlns:w="urn:word"><w:body><w:p><w:r><w:t>Привет</w:t></w:r></w:p><w:p><w:r><w:t>Мир</w:t></w:r></w:p></w:body></w:document>`}))
	if p.Kind != "docx" || !strings.Contains(p.Text, "Привет\nМир") {
		t.Fatalf("DOCX text: %+v", p)
	}
	p = Build("slides.pptx", "", archive(t, map[string]string{
		"ppt/slides/slide10.xml": `<s><p><t>Tenth</t></p></s>`,
		"ppt/slides/slide2.xml":  `<s><p><t>Second</t></p></s>`,
	}))
	if p.Kind != "text" || strings.Index(p.Text, "Second") > strings.Index(p.Text, "Tenth") {
		t.Fatalf("slide order: %+v", p)
	}
	for _, raw := range [][]byte{[]byte("not zip"), archive(t, map[string]string{"word/document.xml": "<broken"}), archive(t, map[string]string{"word/document.xml": strings.Repeat("a", maxXML+1)})} {
		if p := Build("broken.docx", "", raw); p.Kind != "unsupported" {
			t.Fatal("invalid/oversized document accepted")
		}
	}
}

func TestSpreadsheetSheetsSparseCellsAndCachedFormulas(t *testing.T) {
	p := Build("book.xlsx", "", archive(t, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="urn:rels"><sheets><sheet name="План" r:id="r1"/><sheet name="Итог" r:id="r2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="r1" Target="worksheets/sheet1.xml"/><Relationship Id="r2" Target="/xl/worksheets/sheet2.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst><si><r><t>Задача</t></r><r><t> 1</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="C1"><f>1+2</f><v>3</v></c></row><row r="3"><c r="A3" t="inlineStr"><is><t>Привет</t></is></c><c r="BA3"><v>hidden</v></c></row></sheetData></worksheet>`,
		"xl/worksheets/sheet2.xml":   `<worksheet><sheetData><row r="201"><c r="A201"><v>hidden</v></c></row></sheetData></worksheet>`,
	}))
	if p.Kind != "table" || len(p.Sheets) != 2 || !p.Truncated {
		t.Fatalf("XLSX: %+v", p)
	}
	rows := p.Sheets[0].Rows
	if p.Sheets[0].Name != "План" || len(rows) != 3 || len(rows[1]) != 0 || rows[0][0] != "Задача 1" || rows[0][1] != "" || rows[0][2] != "3" || rows[2][0] != "Привет" || len(p.Sheets[1].Rows) != 0 {
		t.Fatalf("worksheet data: %+v", p.Sheets)
	}
}
