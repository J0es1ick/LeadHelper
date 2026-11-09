package filepreview

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// Bound the complete package before letting a browser renderer decompress it.
func ValidateDOCX(data []byte) error {
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return e
	}
	if len(z.File) > 2000 {
		return errors.New("too many document parts")
	}
	remaining, nodes := int64(maxExpanded), 0
	seen := map[string]bool{}
	found := false
	for _, f := range z.File {
		if seen[f.Name] {
			return errors.New("duplicate document part")
		}
		seen[f.Name] = true
		if f.UncompressedSize64 > maxXML || f.UncompressedSize64 > uint64(remaining) {
			return errors.New("document too large")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		b, err := io.ReadAll(io.LimitReader(r, min(int64(maxXML), remaining)+1))
		r.Close()
		remaining -= int64(len(b))
		if err != nil || remaining < 0 || len(b) > maxXML {
			return errors.New("invalid document part")
		}
		if f.Name == "word/document.xml" {
			found = true
		}
		if strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels") {
			d := xml.NewDecoder(bytes.NewReader(b))
			depth := 0
			for {
				token, err := d.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					return err
				}
				switch token.(type) {
				case xml.StartElement:
					depth++
					nodes++
				case xml.EndElement:
					depth--
				}
				if depth > 128 || nodes > 60000 {
					return errors.New("document too complex")
				}
			}
		}
	}
	if !found {
		return errors.New("missing Word document")
	}
	return nil
}
