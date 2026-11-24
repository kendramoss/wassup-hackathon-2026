package ukorigin

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/nporigin"
)

var reSpace = regexp.MustCompile(`[ \t\r\f\v\x{00a0}]+`)

func clean(s string) string { return strings.TrimSpace(reSpace.ReplaceAllString(s, " ")) }

// Blocks reads the body of the HMRC reference document (.docx): its
// paragraphs and tables in document order. A table cell's paragraphs are
// joined by "\n".
func Blocks(docx []byte) ([]nporigin.Block, error) {
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return nil, fmt.Errorf("ukorigin: docx: %w", err)
	}
	var doc []byte
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		if f.UncompressedSize64 > 64<<20 {
			return nil, fmt.Errorf("ukorigin: document.xml is too large")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		doc, err = io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			return nil, err
		}
	}
	if doc == nil {
		return nil, fmt.Errorf("ukorigin: no word/document.xml")
	}
	return readBody(doc)
}

// readBody walks w:body: top-level w:p become paragraphs, w:tbl tables.
// Nested tables are read as text of their cell.
func readBody(doc []byte) ([]nporigin.Block, error) {
	d := xml.NewDecoder(bytes.NewReader(doc))
	var (
		out      []nporigin.Block
		depthTbl int
		para     strings.Builder
		inPara   bool
		rows     [][]string
		row      []string
		cell     []string
	)
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ukorigin: document.xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "tbl":
				depthTbl++
				if depthTbl == 1 {
					rows = nil
				}
			case "tr":
				if depthTbl == 1 {
					row = nil
				}
			case "tc":
				if depthTbl == 1 {
					cell = nil
				}
			case "p":
				inPara = true
				para.Reset()
			case "tab":
				if inPara {
					para.WriteString(" ")
				}
			case "br":
				if inPara {
					para.WriteString(" ")
				}
			}
		case xml.CharData:
			if inPara {
				para.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "p":
				inPara = false
				txt := clean(para.String())
				if depthTbl == 0 {
					out = append(out, nporigin.Block{Text: txt})
				} else if txt != "" {
					cell = append(cell, txt)
				}
			case "tc":
				if depthTbl == 1 {
					row = append(row, strings.Join(cell, "\n"))
				}
			case "tr":
				if depthTbl == 1 {
					rows = append(rows, row)
				}
			case "tbl":
				if depthTbl == 1 {
					out = append(out, nporigin.Block{Rows: rows})
				}
				depthTbl--
			}
		}
	}
	return out, nil
}
