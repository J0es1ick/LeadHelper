// Package filepreview creates bounded, passive previews without external services.
package filepreview

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

const maxText = 1 << 20
const maxXML = 8 << 20
const maxExpanded = 32 << 20
const maxRows = 200
const maxColumns = 50

type Sheet struct {
	HiddenRows []int                `json:"hiddenRows,omitempty"`
	Hidden     bool                 `json:"hidden,omitempty"`
	Name       string               `json:"name"`
	Rows       [][]string           `json:"rows"`
	Cells      map[string]SheetCell `json:"cells,omitempty"`
	Widths     map[int]float64      `json:"widths,omitempty"`
	Heights    map[int]float64      `json:"heights,omitempty"`
	Merges     []SheetMerge         `json:"merges,omitempty"`
	Date1904   bool                 `json:"date1904,omitempty"`
}

type Preview struct {
	Kind      string      `json:"kind"`
	Text      string      `json:"text,omitempty"`
	Sheets    []Sheet     `json:"sheets,omitempty"`
	Notice    string      `json:"notice,omitempty"`
	Truncated bool        `json:"truncated"`
	Styles    []CellStyle `json:"styles,omitempty"`
}

// InlineType ignores the sender's MIME type. Active HTML/SVG is never served inline.
func InlineType(data []byte) string {
	if bytes.HasPrefix(data, []byte("%PDF-")) {
		return "application/pdf"
	}
	switch kind := http.DetectContentType(data); kind {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return kind
	}
	return ""
}

func Build(name, contentType string, data []byte) Preview {
	return build(name, contentType, data, previewLimits)
}

type sheetLimits struct{ rows, columns, sheets, cells, text, cellText int }

var previewLimits = sheetLimits{maxRows, maxColumns, 10, 100000, maxText, 10000}

// ImportSpreadsheet reads a bounded workbook independently of the smaller viewer limits.
// Truncation is reported so the caller can reject a partial import.
func ImportSpreadsheet(name string, data []byte) Preview {
	return build(name, "", data, sheetLimits{5000, 400, 40, 300000, 2 << 20, 60000})
}
func build(name, contentType string, data []byte, limits sheetLimits) Preview {
	maxRows, maxColumns, maxText := limits.rows, limits.columns, limits.text
	if kind := InlineType(data); kind != "" {
		if kind == "application/pdf" {
			return Preview{Kind: "pdf"}
		}
		return Preview{Kind: "image"}
	}
	ext := strings.ToLower(path.Ext(name))
	if ext == ".docx" || ext == ".xlsx" || ext == ".pptx" {
		preview, err := office(ext, data, limits)
		if err != nil {
			return Preview{Kind: "unsupported", Notice: "Не удалось прочитать документ: файл повреждён, защищён паролем или превышает лимит предпросмотра. Скачайте оригинал."}
		}
		return preview
	}
	mediaType, params, _ := mime.ParseMediaType(contentType)
	textFile := strings.HasPrefix(mediaType, "text/") || mediaType == "application/json" || mediaType == "application/xml"
	switch ext {
	case ".txt", ".csv", ".tsv", ".md", ".log", ".json", ".xml", ".html", ".htm", ".svg", ".yaml", ".yml", ".ini":
		textFile = true
	}
	if !textFile {
		return Preview{Kind: "unsupported", Notice: "Для этого формата предпросмотр пока недоступен. Можно скачать оригинал."}
	}
	truncated := len(data) > maxText
	data = data[:min(len(data), maxText)]
	var text string
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}), bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		decoded, err := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder().Bytes(data)
		if err != nil {
			return Preview{Kind: "unsupported", Notice: "Не удалось прочитать кодировку файла. Скачайте оригинал."}
		}
		text = string(decoded)
	case strings.EqualFold(params["charset"], "windows-1251") || strings.EqualFold(params["charset"], "cp1251"):
		decoded, _ := charmap.Windows1251.NewDecoder().Bytes(data)
		text = string(decoded)
	default:
		if bytes.Contains(data, []byte{0}) {
			return Preview{Kind: "unsupported", Notice: "Это двоичный файл. Скачайте оригинал для просмотра."}
		}
		text = strings.ToValidUTF8(string(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})), "�")
	}
	preview := Preview{Kind: "text", Text: text, Truncated: truncated}
	if ext == ".csv" || ext == ".tsv" || mediaType == "text/csv" {
		r := csv.NewReader(strings.NewReader(text))
		r.FieldsPerRecord = -1
		first, _, _ := strings.Cut(text, "\n")
		if ext == ".tsv" {
			r.Comma = '\t'
		} else if strings.Count(first, ";") > strings.Count(first, ",") {
			r.Comma = ';'
		}
		rows := [][]string{}
		cells := 0
		for {
			row, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return preview
			}
			if len(rows) == maxRows {
				preview.Truncated = true
				break
			}
			if len(row) > maxColumns {
				preview.Truncated = true
				row = row[:maxColumns]
			}
			cells += len(row)
			if cells > limits.cells || len(text) > limits.text {
				preview.Truncated = true
				break
			}
			rows = append(rows, row)
		}
		preview.Kind, preview.Text = "table", ""
		preview.Sheets = []Sheet{{Name: name, Rows: rows}}
	}
	return preview
}

type officeArchive struct {
	files     map[string]*zip.File
	remaining int64
}

func (a *officeArchive) read(name string) ([]byte, error) {
	f, ok := a.files[name]
	if !ok {
		return nil, fmt.Errorf("missing document part %s", name)
	}
	if f.UncompressedSize64 > maxXML || f.UncompressedSize64 > uint64(a.remaining) {
		return nil, errors.New("document too large")
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, min(maxXML, a.remaining)+1))
	a.remaining -= int64(len(data))
	if err != nil {
		return nil, err
	}
	if len(data) > maxXML || a.remaining < 0 {
		return nil, errors.New("document too large")
	}
	return data, nil
}

func office(ext string, data []byte, options ...sheetLimits) (Preview, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Preview{}, err
	}
	if len(z.File) > 10000 {
		return Preview{}, errors.New("too many document parts")
	}
	a := &officeArchive{files: map[string]*zip.File{}, remaining: maxExpanded}
	for _, f := range z.File {
		a.files[f.Name] = f
	}
	if ext == ".xlsx" {
		return spreadsheet(a, options...)
	}
	names := []string{"word/document.xml"}
	if ext == ".pptx" {
		names = nil
		for name := range a.files {
			if path.Dir(name) == "ppt/slides" && strings.HasPrefix(path.Base(name), "slide") && strings.HasSuffix(name, ".xml") {
				names = append(names, name)
			}
		}
		sort.Slice(names, func(i, j int) bool { return slideNumber(names[i]) < slideNumber(names[j]) })
		if len(names) == 0 {
			return Preview{}, errors.New("no slides")
		}
	}
	p := Preview{Kind: "text", Notice: "Текст документа. Оформление, изображения и диаграммы доступны в оригинале."}
	var out strings.Builder
	if len(names) > 100 {
		names = names[:100]
		p.Truncated = true
	}
	for n, name := range names {
		raw, err := a.read(name)
		if err != nil {
			return Preview{}, err
		}
		text, err := documentText(raw)
		if err != nil {
			return Preview{}, err
		}
		if ext == ".pptx" {
			fmt.Fprintf(&out, "Слайд %d\n\n", n+1)
		}
		out.WriteString(text)
		out.WriteString("\n")
		if out.Len() > maxText {
			p.Truncated = true
			break
		}
	}
	p.Text = clipText(out.String(), maxText)
	if ext == ".docx" {
		if err := ValidateDOCX(data); err == nil {
			p.Kind = "docx"
			p.Notice = ""
		} else {
			p.Notice = "Документ слишком сложный для просмотра с оформлением. Показан текст; полная версия доступна в оригинале."
		}
	}
	return p, nil
}

func slideNumber(name string) int {
	n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(path.Base(name), "slide"), ".xml"))
	return n
}

func documentText(raw []byte) (string, error) {
	d := xml.NewDecoder(bytes.NewReader(raw))
	var out strings.Builder
	for {
		token, err := d.Token()
		if err == io.EOF {
			return strings.TrimSpace(out.String()), nil
		}
		if err != nil {
			return "", err
		}
		switch v := token.(type) {
		case xml.StartElement:
			if v.Name.Local == "t" {
				var value string
				if err := d.DecodeElement(&value, &v); err != nil {
					return "", err
				}
				out.WriteString(value)
			} else if v.Name.Local == "tab" {
				out.WriteString("\t")
			} else if v.Name.Local == "br" {
				out.WriteString("\n")
			}
		case xml.EndElement:
			if v.Name.Local == "p" {
				out.WriteString("\n")
			} else if v.Name.Local == "tc" {
				out.WriteString("\t")
			}
		}
	}
}

func clipText(value string, n int) string {
	if len(value) <= n {
		return value
	}
	for n > 0 && !utf8.RuneStart(value[n]) {
		n--
	}
	return value[:n]
}

func spreadsheet(a *officeArchive, options ...sheetLimits) (Preview, error) {
	limits := previewLimits
	if len(options) > 0 {
		limits = options[0]
	}
	maxRows, maxColumns := limits.rows, limits.columns
	allocated := 0
	workbook, err := a.read("xl/workbook.xml")
	if err != nil {
		return Preview{}, err
	}
	var book struct {
		Properties struct {
			Date1904 bool `xml:"date1904,attr"`
		} `xml:"workbookPr"`
		Sheets []struct {
			State string `xml:"state,attr"`
			Name  string `xml:"name,attr"`
			ID    string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err = xml.Unmarshal(workbook, &book); err != nil {
		return Preview{}, err
	}
	rels, err := a.read("xl/_rels/workbook.xml.rels")
	if err != nil {
		return Preview{}, err
	}
	var relationships struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if err = xml.Unmarshal(rels, &relationships); err != nil {
		return Preview{}, err
	}
	targets := map[string]string{}
	for _, r := range relationships.Items {
		if r.Mode != "External" {
			target := path.Clean(path.Join("xl", r.Target))
			if strings.HasPrefix(r.Target, "/") {
				target = strings.TrimPrefix(path.Clean(r.Target), "/")
			}
			if strings.HasPrefix(target, "xl/") {
				targets[r.ID] = target
			}
		}
	}
	shared := []string{}
	if _, ok := a.files["xl/sharedStrings.xml"]; ok {
		raw, err := a.read("xl/sharedStrings.xml")
		if err != nil {
			return Preview{}, err
		}
		d := xml.NewDecoder(bytes.NewReader(raw))
		for {
			token, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return Preview{}, err
			}
			if element, ok := token.(xml.StartElement); ok && element.Name.Local == "si" {
				var item struct {
					Inner string `xml:",innerxml"`
				}
				if err := d.DecodeElement(&item, &element); err != nil {
					return Preview{}, err
				}
				text, err := documentText([]byte("<root>" + item.Inner + "</root>"))
				if err != nil {
					return Preview{}, err
				}
				shared = append(shared, text)
			}
		}
	}
	p := Preview{Kind: "table", Sheets: []Sheet{}, Notice: "Первые 200 строк и 50 столбцов каждого из первых 10 листов. Формулы не пересчитываются: показаны сохранённые значения. Диаграммы и условное форматирование доступны в оригинале."}
	if limits.rows != previewLimits.rows {
		p.Notice = "Формулы не пересчитываются: импортируются сохранённые значения."
	}
	if _, ok := a.files["xl/styles.xml"]; ok {
		p.Styles, err = spreadsheetStyles(a)
		if err != nil {
			return Preview{}, err
		}
	}
	textBudget := limits.text
	if len(book.Sheets) > limits.sheets {
		book.Sheets = book.Sheets[:limits.sheets]
		p.Truncated = true
	}
	for _, sheet := range book.Sheets {
		if textBudget == 0 {
			p.Truncated = true
			break
		}
		raw, err := a.read(targets[sheet.ID])
		if err != nil {
			return Preview{}, err
		}
		var document struct {
			Columns []struct {
				Min   int     `xml:"min,attr"`
				Max   int     `xml:"max,attr"`
				Width float64 `xml:"width,attr"`
			} `xml:"cols>col"`
			Merges []struct {
				Ref string `xml:"ref,attr"`
			} `xml:"mergeCells>mergeCell"`
			Rows []struct {
				Hidden bool    `xml:"hidden,attr"`
				Number int     `xml:"r,attr"`
				Height float64 `xml:"ht,attr"`
				Cells  []struct {
					Style   int    `xml:"s,attr"`
					Formula string `xml:"f"`
					Ref     string `xml:"r,attr"`
					Type    string `xml:"t,attr"`
					Value   string `xml:"v"`
					Inline  struct {
						Inner string `xml:",innerxml"`
					} `xml:"is"`
				} `xml:"c"`
			} `xml:"sheetData>row"`
		}
		if err := xml.Unmarshal(raw, &document); err != nil {
			return Preview{}, err
		}
		rows := [][]string{}
		styled := Sheet{Name: sheet.Name, Cells: map[string]SheetCell{}, Widths: map[int]float64{}, Heights: map[int]float64{}, Date1904: book.Properties.Date1904}
		styled.Hidden = sheet.State == "hidden" || sheet.State == "veryHidden"
		for _, col := range document.Columns {
			for i := max(1, col.Min); i <= min(maxColumns, col.Max); i++ {
				if col.Width > 0 {
					styled.Widths[i-1] = min(500, max(40, col.Width*7+5))
				}
			}
		}
		for _, merge := range document.Merges {
			if len(styled.Merges) >= 1000 {
				p.Truncated = true
				break
			}
			if m, ok := sheetMerge(merge.Ref); ok {
				styled.Merges = append(styled.Merges, m)
			}
		}
		for _, row := range document.Rows {
			if textBudget == 0 {
				p.Truncated = true
				break
			}
			n := row.Number
			if n == 0 {
				n = len(rows) + 1
			}
			if n > maxRows {
				p.Truncated = true
				break
			}
			if n < 1 {
				continue
			}
			if row.Hidden {
				styled.HiddenRows = append(styled.HiddenRows, n-1)
			}
			if row.Height > 0 {
				styled.Heights[n-1] = min(400, max(16, row.Height*4/3))
			}
			for len(rows) < n {
				rows = append(rows, []string{})
			}
			values := rows[n-1]
			for index, cell := range row.Cells {
				if textBudget == 0 {
					p.Truncated = true
					break
				}
				col := columnLimit(cell.Ref, maxColumns)
				if cell.Ref == "" {
					col = index
				}
				if col < 0 || col >= maxColumns {
					p.Truncated = true
					continue
				}
				value := cell.Value
				switch cell.Type {
				case "s":
					index, err := strconv.Atoi(value)
					if err != nil || index < 0 || index >= len(shared) {
						return Preview{}, errors.New("invalid shared string")
					}
					value = shared[index]
				case "inlineStr":
					value, err = documentText([]byte("<root>" + cell.Inline.Inner + "</root>"))
					if err != nil {
						return Preview{}, err
					}
				case "b":
					if value == "1" {
						value = "TRUE"
					} else {
						value = "FALSE"
					}
				}
				needed := max(0, col+1-len(values))
				allocated += needed
				if allocated > limits.cells {
					return Preview{}, errors.New("too many spreadsheet cells")
				}
				for len(values) <= col {
					values = append(values, "")
				}
				limit := min(limits.cellText, textBudget)
				if len(value) > limit {
					p.Truncated = true
				}
				values[col] = clipText(value, limit)
				format := SheetCell{Style: cell.Style, Formula: clipText(cell.Formula, 1000)}
				if cell.Type == "" || cell.Type == "n" {
					if number, e := strconv.ParseFloat(cell.Value, 64); e == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
						format.Number = &number
					}
				}
				styled.Cells[fmt.Sprintf("%d:%d", n-1, col)] = format
				textBudget -= len(values[col])
				if limit > 0 && len(values[col]) == 0 && len(value) > 0 {
					textBudget = 0
				}
			}
			rows[n-1] = values
		}
		styled.Rows = rows
		p.Sheets = append(p.Sheets, styled)
	}
	if len(p.Sheets) == 0 {
		return Preview{}, errors.New("no worksheets")
	}
	return p, nil
}

func column(ref string) int {
	return columnLimit(ref, maxColumns)
}
func columnLimit(ref string, limit int) int {
	n := 0
	for _, c := range ref {
		if c < 'A' || c > 'Z' {
			break
		}
		n = n*26 + int(c-'A'+1)
		if n > limit {
			return limit
		}
	}
	return n - 1
}
