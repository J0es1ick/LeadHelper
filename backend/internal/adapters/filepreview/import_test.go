package filepreview

import (
	"strings"
	"testing"
)

func TestImportWorkbookLimitsAreIndependentOfPreview(t *testing.T) {
	data := archive(t, map[string]string{
		"xl/workbook.xml":            `<workbook xmlns:r="r"><sheets><sheet name="Presale" r:id="one"/><sheet name="Hidden" state="hidden" r:id="two"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="one" Target="worksheets/one.xml"/><Relationship Id="two" Target="worksheets/two.xml"/></Relationships>`,
		"xl/worksheets/one.xml":      `<worksheet><sheetData><row r="201" hidden="1"><c r="DU201" t="inlineStr"><is><t>Last status</t></is></c></row></sheetData></worksheet>`,
		"xl/worksheets/two.xml":      `<worksheet><sheetData><row><c t="inlineStr"><is><t>Secret draft</t></is></c></row></sheetData></worksheet>`,
	})
	p := ImportSpreadsheet("projects.xlsx", data)
	if len(p.Sheets[0].HiddenRows) != 1 || p.Sheets[0].HiddenRows[0] != 200 {
		t.Fatal("hidden row metadata lost")
	}
	if p.Kind != "table" || p.Truncated || len(p.Sheets) != 2 || !p.Sheets[1].Hidden || p.Sheets[0].Rows[200][124] != "Last status" {
		t.Fatalf("incomplete import: %+v", p)
	}
	if viewer := Build("projects.xlsx", "", data); !viewer.Truncated {
		t.Fatal("viewer should keep its smaller bounds")
	}
	if p = ImportSpreadsheet("large.csv", []byte(strings.Repeat("name,status\n", 5000))); p.Truncated || len(p.Sheets[0].Rows) != 5000 {
		t.Fatal("bounded CSV import rejected")
	}
	if p = ImportSpreadsheet("too-large.csv", []byte(strings.Repeat("name,status\n", 5001))); !p.Truncated {
		t.Fatal("row bound ignored")
	}
	if p = ImportSpreadsheet("too-wide.csv", []byte(strings.Repeat("cell,", 400)+"last\n")); !p.Truncated {
		t.Fatal("column bound ignored")
	}
	if p = ImportSpreadsheet("too-many-cells.csv", []byte(strings.Repeat(strings.Repeat("a,", 399)+"a\n", 751))); !p.Truncated {
		t.Fatal("cell bound ignored")
	}
}
