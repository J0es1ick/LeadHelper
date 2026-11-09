package filepreview

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

type SheetCell struct {
	Style   int      `json:"style"`
	Number  *float64 `json:"number,omitempty"`
	Formula string   `json:"formula,omitempty"`
}
type SheetMerge struct {
	Row  int `json:"row"`
	Col  int `json:"col"`
	Rows int `json:"rows"`
	Cols int `json:"cols"`
}
type CellStyle struct {
	Font      string            `json:"font,omitempty"`
	Size      float64           `json:"size,omitempty"`
	Bold      bool              `json:"bold,omitempty"`
	Italic    bool              `json:"italic,omitempty"`
	Underline bool              `json:"underline,omitempty"`
	Color     string            `json:"color,omitempty"`
	Fill      string            `json:"fill,omitempty"`
	Align     string            `json:"align,omitempty"`
	Vertical  string            `json:"vertical,omitempty"`
	Wrap      bool              `json:"wrap,omitempty"`
	Borders   map[string]string `json:"borders,omitempty"`
	Format    string            `json:"format,omitempty"`
	FormatID  int               `json:"formatId,omitempty"`
}
type xColor struct {
	RGB     string `xml:"rgb,attr"`
	Theme   *int   `xml:"theme,attr"`
	Indexed *int   `xml:"indexed,attr"`
}
type xBorder struct {
	Style string `xml:"style,attr"`
	Color xColor `xml:"color"`
}

func spreadsheetStyles(a *officeArchive) ([]CellStyle, error) {
	raw, e := a.read("xl/styles.xml")
	if e != nil {
		return nil, e
	}
	var doc struct {
		Formats []struct {
			ID   int    `xml:"numFmtId,attr"`
			Code string `xml:"formatCode,attr"`
		} `xml:"numFmts>numFmt"`
		Fonts []struct {
			Name struct {
				Value string `xml:"val,attr"`
			} `xml:"name"`
			Size struct {
				Value float64 `xml:"val,attr"`
			} `xml:"sz"`
			Bold *struct {
				Value string `xml:"val,attr"`
			} `xml:"b"`
			Italic *struct {
				Value string `xml:"val,attr"`
			} `xml:"i"`
			Underline *struct{} `xml:"u"`
			Color     xColor    `xml:"color"`
		} `xml:"fonts>font"`
		Fills []struct {
			Pattern struct {
				Type  string `xml:"patternType,attr"`
				Color xColor `xml:"fgColor"`
			} `xml:"patternFill"`
		} `xml:"fills>fill"`
		Borders []struct {
			Top    xBorder `xml:"top"`
			Bottom xBorder `xml:"bottom"`
			Left   xBorder `xml:"left"`
			Right  xBorder `xml:"right"`
		} `xml:"borders>border"`
		Styles []struct {
			Font      int `xml:"fontId,attr"`
			Fill      int `xml:"fillId,attr"`
			Border    int `xml:"borderId,attr"`
			Format    int `xml:"numFmtId,attr"`
			Alignment struct {
				Horizontal string `xml:"horizontal,attr"`
				Vertical   string `xml:"vertical,attr"`
				Wrap       bool   `xml:"wrapText,attr"`
			} `xml:"alignment"`
		} `xml:"cellXfs>xf"`
	}
	if e = xml.Unmarshal(raw, &doc); e != nil {
		return nil, e
	}
	theme := []string{"FFFFFF", "000000", "EEECE1", "1F497D", "4F81BD", "C0504D", "9BBB59", "8064A2", "4BACC6", "F79646"}
	if _, ok := a.files["xl/theme/theme1.xml"]; ok {
		b, e := a.read("xl/theme/theme1.xml")
		if e != nil {
			return nil, e
		}
		var t struct {
			Scheme struct {
				Nodes []struct {
					Name  xml.Name
					Color struct {
						Val string `xml:"val,attr"`
					} `xml:"srgbClr"`
					System struct {
						Val string `xml:"lastClr,attr"`
					} `xml:"sysClr"`
				} `xml:",any"`
			} `xml:"themeElements>clrScheme"`
		}
		if xml.Unmarshal(b, &t) == nil {
			slots := map[string]int{"lt1": 0, "dk1": 1, "lt2": 2, "dk2": 3, "accent1": 4, "accent2": 5, "accent3": 6, "accent4": 7, "accent5": 8, "accent6": 9}
			for _, n := range t.Scheme.Nodes {
				if i, ok := slots[n.Name.Local]; ok {
					v := n.Color.Val
					if v == "" {
						v = n.System.Val
					}
					theme[i] = v
				}
			}
		}
	}
	color := func(c xColor) string {
		v := c.RGB
		if c.Theme != nil && *c.Theme >= 0 && *c.Theme < len(theme) {
			v = theme[*c.Theme]
		}
		palette := []string{"000000", "FFFFFF", "FF0000", "00FF00", "0000FF", "FFFF00", "FF00FF", "00FFFF"}
		if c.Indexed != nil && *c.Indexed >= 0 && *c.Indexed < 64 {
			v = palette[*c.Indexed%8]
		}
		if len(v) == 8 {
			v = v[2:]
		}
		if len(v) != 6 {
			return ""
		}
		if _, e := strconv.ParseUint(v, 16, 24); e != nil {
			return ""
		}
		return "#" + v
	}
	formats := map[int]string{}
	for _, f := range doc.Formats {
		formats[f.ID] = clipText(f.Code, 200)
	}
	out := []CellStyle{}
	for _, x := range doc.Styles[:min(len(doc.Styles), 512)] {
		s := CellStyle{Format: formats[x.Format], FormatID: x.Format, Borders: map[string]string{}, Align: x.Alignment.Horizontal, Vertical: x.Alignment.Vertical, Wrap: x.Alignment.Wrap}
		if x.Font >= 0 && x.Font < len(doc.Fonts) {
			f := doc.Fonts[x.Font]
			s.Font = clipText(f.Name.Value, 80)
			s.Size = min(72, max(8, f.Size.Value))
			s.Bold = f.Bold != nil && f.Bold.Value != "0" && f.Bold.Value != "false"
			s.Italic = f.Italic != nil && f.Italic.Value != "0" && f.Italic.Value != "false"
			s.Underline = f.Underline != nil
			s.Color = color(f.Color)
		}
		if x.Fill >= 0 && x.Fill < len(doc.Fills) && doc.Fills[x.Fill].Pattern.Type == "solid" {
			s.Fill = color(doc.Fills[x.Fill].Pattern.Color)
		}
		if x.Border >= 0 && x.Border < len(doc.Borders) {
			b := doc.Borders[x.Border]
			for side, v := range map[string]xBorder{"top": b.Top, "bottom": b.Bottom, "left": b.Left, "right": b.Right} {
				if v.Style != "" && v.Style != "none" {
					width, style := 1, "solid"
					if v.Style == "medium" {
						width = 2
					}
					if v.Style == "thick" {
						width = 3
					}
					if strings.Contains(strings.ToLower(v.Style), "dash") {
						style = "dashed"
					}
					if v.Style == "dotted" {
						style = "dotted"
					}
					if v.Style == "double" {
						width = 3
						style = "double"
					}
					c := color(v.Color)
					if c == "" {
						c = "#333333"
					}
					s.Borders[side] = fmt.Sprintf("%dpx %s %s", width, style, c)
				}
			}
		}
		out = append(out, s)
	}
	return out, nil
}
func sheetMerge(ref string) (SheetMerge, bool) {
	parts := strings.Split(ref, ":")
	if len(parts) != 2 {
		return SheetMerge{}, false
	}
	parse := func(s string) (int, int) {
		c := column(s)
		digits := strings.TrimLeft(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
		r, e := strconv.Atoi(digits)
		if e != nil {
			return -1, -1
		}
		return r - 1, c
	}
	r, c := parse(parts[0])
	z, k := parse(parts[1])
	if r < 0 || c < 0 || r >= maxRows || c >= maxColumns || z < r || k < c {
		return SheetMerge{}, false
	}
	return SheetMerge{Row: r, Col: c, Rows: min(maxRows-1, z) - r + 1, Cols: min(maxColumns-1, k) - c + 1}, true
}
