package attachment

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"
)

const maxRows = 100

// Extract returns the textual representation of a file.
// Images return empty string — they are handled separately as multimodal content.
func Extract(info *FileInfo) (string, error) {
	switch info.Kind {
	case "text":
		return string(info.Content), nil
	case "excel":
		return extractExcel(info.Content)
	case "pdf":
		return extractPDF(info.Content, int64(len(info.Content)))
	case "word":
		return extractDocx(info.Content)
	case "image":
		return "", nil
	}
	return "", nil
}

func extractExcel(data []byte) (string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("无法读取 Excel 文件: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n## Sheet: %s\n\n", sheet))
		truncated := false
		for i, row := range rows {
			if i >= maxRows {
				truncated = true
				break
			}
			sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
			if i == 0 {
				sep := make([]string, len(row))
				for j := range sep {
					sep[j] = "---"
				}
				sb.WriteString("| " + strings.Join(sep, " | ") + " |\n")
			}
		}
		if truncated {
			sb.WriteString(fmt.Sprintf("\n*(文件过大，已截取前 %d 行)*\n", maxRows))
		}
	}
	return sb.String(), nil
}

func extractPDF(data []byte, size int64) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), size)
	if err != nil {
		return "", fmt.Errorf("无法读取 PDF 文件: %w", err)
	}
	var sb strings.Builder
	numPages := r.NumPage()
	if numPages > maxRows {
		numPages = maxRows
	}
	for i := 1; i <= numPages; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		text, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}
	if r.NumPage() > maxRows {
		sb.WriteString(fmt.Sprintf("\n*(文件过大，已截取前 %d 页)*\n", maxRows))
	}
	return sb.String(), nil
}

// extractDocx extracts text from a .docx file.
// A .docx is a ZIP archive; the main text lives in word/document.xml as <w:t> elements.
func extractDocx(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("无法读取 Word 文件: %w", err)
	}

	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()
			return parseDocumentXML(rc)
		}
	}
	return "", fmt.Errorf("无法读取文件，请确认文件未损坏")
}

// parseDocumentXML walks the XML and extracts text from <w:t> elements,
// inserting newlines at paragraph boundaries (<w:p>).
func parseDocumentXML(r io.Reader) (string, error) {
	decoder := xml.NewDecoder(r)
	var sb strings.Builder
	inParagraph := false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return sb.String(), nil // return what we have
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" && t.Name.Space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" {
				if inParagraph {
					sb.WriteString("\n")
				}
				inParagraph = true
			}
			if t.Name.Local == "t" && t.Name.Space == "http://schemas.openxmlformats.org/wordprocessingml/2006/main" {
				var text string
				if err := decoder.DecodeElement(&text, &t); err == nil {
					sb.WriteString(text)
				}
			}
		}
	}
	return sb.String(), nil
}
