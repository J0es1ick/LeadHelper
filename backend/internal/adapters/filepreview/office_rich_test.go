package filepreview

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDOCXFullPackageLimits(t *testing.T) {
	raw := archive(t, map[string]string{"word/document.xml": "<document><p><t>Text</t></p></document>", "word/media/large.png": strings.Repeat("x", maxXML+1)})
	if ValidateDOCX(raw) == nil {
		t.Fatal("oversize image accepted")
	}
	if p := Build("doc.docx", "", raw); p.Kind != "text" || p.Notice == "" {
		t.Fatal("text fallback missing", p)
	}
	raw = archive(t, map[string]string{"word/document.xml": "<doc>" + strings.Repeat("<x/>", 60001) + "</doc>"})
	if ValidateDOCX(raw) == nil {
		t.Fatal("excess render nodes accepted")
	}
}
func TestSpreadsheetStylesMergesWidthsAndSafeNumbers(t *testing.T) {
	p := Build("test.xlsx", "", archive(t, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="urn:rels"><workbookPr date1904="1"/><sheets><sheet name="План" r:id="r1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="r1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/styles.xml":              `<styleSheet><numFmts><numFmt numFmtId="164" formatCode="dd.mm.yyyy"/></numFmts><fonts><font><name val="Arial"/><sz val="12"/><b/><color rgb="FF123456"/></font></fonts><fills><fill><patternFill patternType="solid"><fgColor rgb="FFF0F8F0"/></patternFill></fill></fills><borders><border><bottom style="thin"><color rgb="FF334455"/></bottom></border></borders><cellXfs><xf fontId="0" fillId="0" borderId="0" numFmtId="164"><alignment horizontal="center" wrapText="1"/></xf></cellXfs></styleSheet>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><cols><col min="1" max="2" width="24"/></cols><sheetData><row r="1" ht="30"><c r="A1"><v>45000</v></c></row><row r="2"><c r="A2"><v>NaN</v></c></row></sheetData><mergeCells><mergeCell ref="A1:B1"/></mergeCells></worksheet>`,
	}))
	if p.Kind != "table" || len(p.Styles) != 1 || !p.Styles[0].Bold || p.Styles[0].Fill != "#F0F8F0" || p.Styles[0].Borders["bottom"] != "1px solid #334455" {
		t.Fatal(p)
	}
	s := p.Sheets[0]
	if !s.Date1904 || s.Widths[0] != 173 || s.Heights[0] != 40 || s.Merges[0].Cols != 2 || *s.Cells["0:0"].Number != 45000 || s.Cells["1:0"].Number != nil {
		t.Fatal(s)
	}
	if _, e := json.Marshal(p); e != nil {
		t.Fatal(e)
	}
}
