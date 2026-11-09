package xlsxexport

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"leadhelper/internal/domain"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWorklogWorkbookIsNumericAndHasSafeTextAndCachedTotals(t *testing.T) {
	report := domain.WorklogExportReport{From: "2026-11-09", To: "2026-11-15", GeneratedAt: time.Now(), Missing: 1, Total: 19860, Periods: []domain.WorklogPeriod{{From: "2026-11-09", To: "2026-11-11"}, {From: "2026-11-12", To: "2026-11-15"}}, Rows: []domain.WorklogExportRow{
		{Name: "=HYPERLINK(\"https://example.test\") & <Иванова>", Available: true, Seconds: []int64{9000, 10860}, Total: 19860},
		{Name: "Нет сопоставления", Seconds: []int64{0, 0}}, {Name: "Без списаний", Available: true, Seconds: []int64{0, 0}},
	}}
	data, e := Worklogs(report)
	if e != nil {
		t.Fatal(e)
	}
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		t.Fatal(e)
	}
	parts := map[string]string{}
	for _, f := range z.File {
		r, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(r)
		r.Close()
		if e != nil {
			t.Fatal(e)
		}
		var value any
		// Decode every part to ensure no source string can break the document XML.
		if e = xml.Unmarshal(b, &value); e != nil {
			t.Fatal(f.Name, e)
		}
		parts[f.Name] = string(b)
	}
	var sheet struct {
		Rows []struct {
			Cells []struct {
				Ref     string `xml:"r,attr"`
				Type    string `xml:"t,attr"`
				Value   string `xml:"v"`
				Formula string `xml:"f"`
				Text    string `xml:"is>t"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if e = xml.Unmarshal([]byte(parts["xl/worksheets/sheet1.xml"]), &sheet); e != nil {
		t.Fatal(e)
	}
	numbers := map[string]float64{}
	formulas := map[string]string{}
	texts := map[string]string{}
	for _, row := range sheet.Rows {
		for _, c := range row.Cells {
			if c.Value != "" {
				numbers[c.Ref], _ = strconv.ParseFloat(c.Value, 64)
			}
			if c.Formula != "" {
				formulas[c.Ref] = c.Formula
			}
			if c.Type == "inlineStr" {
				texts[c.Ref] = c.Text
			}
		}
	}
	if math.Abs(numbers["B8"]-5.516666666666667) > 1e-12 || numbers["C8"] != 2.5 || math.Abs(numbers["D8"]-3.0166666666666666) > 1e-12 {
		t.Fatal(numbers)
	}
	if _, exists := numbers["B9"]; exists {
		t.Fatal("unknown hours written as zero")
	}
	if value, exists := numbers["B10"]; !exists || value != 0 {
		t.Fatal("known zero not numeric")
	}
	if formulas["B8"] != "SUM(C8:D8)" || formulas["B11"] != "SUM(B8:B10)" || texts["A8"] != report.Rows[0].Name {
		t.Fatal(formulas, texts)
	}
	if !strings.Contains(parts["xl/worksheets/sheet1.xml"], `state="frozen"`) || !strings.Contains(parts["xl/styles.xml"], `formatCode="0.00"`) {
		t.Fatal("missing presentation features")
	}
}
func TestWholePeriodDoesNotRepeatTheSameTotal(t *testing.T) {
	report := domain.WorklogExportReport{From: "2026-11-09", To: "2026-11-15", GeneratedAt: time.Now(), Periods: []domain.WorklogPeriod{{From: "2026-11-09", To: "2026-11-15"}}, Rows: []domain.WorklogExportRow{{Name: "Сотрудник", Available: true, Seconds: []int64{3600}, Total: 3600}}, Total: 3600}
	data, e := Worklogs(report)
	if e != nil {
		t.Fatal(e)
	}
	z, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	for _, f := range z.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			r, _ := f.Open()
			b, _ := io.ReadAll(r)
			r.Close()
			if !bytes.Contains(b, []byte(`ref="A1:C9"`)) {
				t.Fatal(string(b))
			}
		}
	}
}
