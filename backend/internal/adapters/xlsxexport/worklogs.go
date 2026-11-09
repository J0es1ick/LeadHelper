// Package xlsxexport writes portable XLSX reports without Office or a separate
// document server. User/source text is always an inline string, never a formula.
package xlsxexport

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"leadhelper/internal/domain"
	"strconv"
	"strings"
	"time"
)

const declaration = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
const ns = `http://schemas.openxmlformats.org/spreadsheetml/2006/main`

func column(n int) string {
	out := ""
	for n > 0 {
		n--
		out = string(rune('A'+n%26)) + out
		n /= 26
	}
	return out
}
func escape(v string) string {
	v = strings.Map(func(r rune) rune {
		if r == 9 || r == 10 || r == 13 || r >= 32 && r <= 0xD7FF || r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF {
			return r
		}
		return -1
	}, v)
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(v))
	return b.String()
}
func Worklogs(report domain.WorklogExportReport) ([]byte, error) {
	if len(report.Rows) == 0 || len(report.Rows) > 500 || len(report.Periods) == 0 || len(report.Periods) > 366 {
		return nil, fmt.Errorf("invalid report dimensions")
	}
	split := len(report.Periods) > 1
	cols := 3
	if split {
		cols += len(report.Periods)
	}
	lastCol := column(cols)
	firstRow := 8
	lastRow := firstRow + len(report.Rows) - 1
	totalRow := lastRow + 1
	var sheet strings.Builder
	fmt.Fprintf(&sheet, `%s<worksheet xmlns="%s"><dimension ref="A1:%s%d"/><sheetViews><sheetView workbookViewId="0" showGridLines="0"><pane xSplit="1" ySplit="7" topLeftCell="B8" activePane="bottomRight" state="frozen"/><selection pane="bottomRight" activeCell="B8" sqref="B8"/></sheetView></sheetViews><sheetFormatPr defaultRowHeight="22"/><cols><col min="1" max="1" width="34" customWidth="1"/><col min="2" max="2" width="15" customWidth="1"/>`, declaration, ns, lastCol, totalRow)
	if split {
		fmt.Fprintf(&sheet, `<col min="3" max="%d" width="18" customWidth="1"/>`, cols-1)
	}
	fmt.Fprintf(&sheet, `<col min="%d" max="%d" width="29" customWidth="1"/></cols><sheetData>`, cols, cols)
	textCell := func(col, row, style int, value string) {
		fmt.Fprintf(&sheet, `<c r="%s%d" s="%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, column(col), row, style, escape(value))
	}
	numberCell := func(col, row, style int, seconds int64, formula string) {
		fmt.Fprintf(&sheet, `<c r="%s%d" s="%d">`, column(col), row, style)
		if formula != "" {
			fmt.Fprintf(&sheet, `<f>%s</f>`, formula)
		}
		fmt.Fprintf(&sheet, `<v>%s</v></c>`, strconv.FormatFloat(float64(seconds)/3600, 'f', -1, 64))
	}
	dateLabel := func(value string) string { d, _ := time.Parse("2006-01-02", value); return d.Format("02.01.2006") }
	note := fmt.Sprintf("Сотрудников: %d. Часы по автору списания и дате работы в Jira (Москва).", len(report.Rows))
	coverage := "Ноль означает отсутствие списаний за выбранные даты. Часы в каждом подпериоде считаются отдельно."
	if report.Missing > 0 {
		coverage = fmt.Sprintf("Нет данных у %d сотрудников без сопоставления с Jira. Их ячейки пусты; итог включает только доступные данные.", report.Missing)
	}
	loc, _ := time.LoadLocation("Europe/Moscow")
	info := []struct {
		text   string
		style  int
		height int
	}{
		{"Затраченное время команды", 1, 34},
		{"Период: " + dateLabel(report.From) + " — " + dateLabel(report.To) + " включительно. Единица: десятичные часы.", 2, 25},
		{"Источник: Jira. Выгружено " + report.GeneratedAt.In(loc).Format("02.01.2006 15:04") + " МСК.", 2, 25},
		{note, 2, 30}, {coverage, 2, 42},
	}
	for i, r := range info {
		fmt.Fprintf(&sheet, `<row r="%d" ht="%d" customHeight="1">`, i+1, r.height)
		textCell(1, i+1, r.style, r.text)
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`<row r="7" ht="44" customHeight="1">`)
	textCell(1, 7, 3, "Сотрудник")
	textCell(2, 7, 3, "Итого, ч")
	if split {
		for i, p := range report.Periods {
			textCell(i+3, 7, 3, dateLabel(p.From)+" —\n"+dateLabel(p.To)+", ч")
		}
	}
	textCell(cols, 7, 3, "Данные")
	sheet.WriteString(`</row>`)
	totals := make([]int64, len(report.Periods))
	for i, r := range report.Rows {
		if len(r.Seconds) != len(report.Periods) {
			return nil, fmt.Errorf("invalid row dimensions")
		}
		row := firstRow + i
		fmt.Fprintf(&sheet, `<row r="%d" ht="30" customHeight="1">`, row)
		textCell(1, row, 4, r.Name)
		if r.Available {
			formula := ""
			if split {
				formula = fmt.Sprintf("SUM(C%d:%s%d)", row, column(cols-1), row)
			}
			numberCell(2, row, 6, r.Total, formula)
			if split {
				for j, seconds := range r.Seconds {
					numberCell(j+3, row, 5, seconds, "")
					totals[j] += seconds
				}
			}
			textCell(cols, row, 4, "Jira")
		} else {
			textCell(cols, row, 8, "Нет сопоставления с Jira")
		}
		sheet.WriteString(`</row>`)
	}
	fmt.Fprintf(&sheet, `<row r="%d" ht="30" customHeight="1">`, totalRow)
	textCell(1, totalRow, 7, "Итого по сотрудникам")
	numberCell(2, totalRow, 6, report.Total, fmt.Sprintf("SUM(B%d:B%d)", firstRow, lastRow))
	if split {
		for j, seconds := range totals {
			col := column(j + 3)
			numberCell(j+3, totalRow, 6, seconds, fmt.Sprintf("SUM(%s%d:%s%d)", col, firstRow, col, lastRow))
		}
	}
	textCell(cols, totalRow, 7, fmt.Sprintf("С данными: %d из %d", len(report.Rows)-report.Missing, len(report.Rows)))
	fmt.Fprintf(&sheet, `</row></sheetData><autoFilter ref="A7:%s%d"/><mergeCells count="5">`, lastCol, lastRow)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&sheet, `<mergeCell ref="A%d:%s%d"/>`, i, lastCol, i)
	}
	sheet.WriteString(`</mergeCells><printOptions horizontalCentered="0"/><pageMargins left="0.3" right="0.3" top="0.4" bottom="0.4" header="0.2" footer="0.2"/><pageSetup paperSize="9" orientation="landscape" fitToWidth="1" fitToHeight="0"/></worksheet>`)
	styles := declaration + `<styleSheet xmlns="` + ns + `"><numFmts count="1"><numFmt numFmtId="164" formatCode="0.00"/></numFmts><fonts count="4"><font><sz val="11"/><name val="Calibri"/><color rgb="FF243B32"/></font><font><b/><sz val="18"/><name val="Calibri"/><color rgb="FF285B48"/></font><font><b/><sz val="11"/><name val="Calibri"/><color rgb="FFFFFFFF"/></font><font><b/><sz val="11"/><name val="Calibri"/><color rgb="FF243B32"/></font></fonts><fills count="4"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF356650"/><bgColor indexed="64"/></patternFill></fill><fill><patternFill patternType="solid"><fgColor rgb="FFEAF2EC"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="2"><border/><border><bottom style="hair"><color rgb="FFDCE5DF"/></bottom></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="9"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment vertical="center"/></xf><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf><xf numFmtId="0" fontId="2" fillId="2" borderId="0" xfId="0" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf><xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf><xf numFmtId="164" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyAlignment="1"><alignment horizontal="right" vertical="center"/></xf><xf numFmtId="164" fontId="3" fillId="3" borderId="1" xfId="0" applyNumberFormat="1" applyAlignment="1"><alignment horizontal="right" vertical="center"/></xf><xf numFmtId="0" fontId="3" fillId="3" borderId="1" xfId="0" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf><xf numFmtId="0" fontId="0" fillId="3" borderId="1" xfId="0" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`
	parts := []struct{ name, value string }{
		{"[Content_Types].xml", declaration + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", declaration + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", declaration + `<workbook xmlns="` + ns + `" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><bookViews><workbookView/></bookViews><sheets><sheet name="Часы сотрудников" sheetId="1" r:id="rId1"/></sheets><definedNames><definedName name="_xlnm.Print_Titles" localSheetId="0">'Часы сотрудников'!$1:$7</definedName></definedNames><calcPr calcId="191029" fullCalcOnLoad="1"/></workbook>`},
		{"xl/_rels/workbook.xml.rels", declaration + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", styles}, {"xl/worksheets/sheet1.xml", sheet.String()},
	}
	var output bytes.Buffer
	z := zip.NewWriter(&output)
	for _, part := range parts {
		w, e := z.Create(part.name)
		if e != nil {
			return nil, e
		}
		if _, e = w.Write([]byte(part.value)); e != nil {
			return nil, e
		}
	}
	if e := z.Close(); e != nil {
		return nil, e
	}
	return output.Bytes(), nil
}
