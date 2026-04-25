// Package chat (infra/chat) provides file content extraction for chat
// attachments. Each supported format has a dedicated extractor; the
// top-level Extract function dispatches by MIME type and file extension.
//
// Build note: the PDF extractor uses dslipak/pdf (pure Go). Excel uses
// xuri/excelize/v2 (pure Go). DOCX/ODT/RTF use lu4p/cat (pure Go). PPTX
// is parsed directly from the ZIP/XML structure with stdlib only.
//
// Package chat（infra/chat）为聊天附件提供文件内容提取。每种支持的格式有
// 对应的提取器；顶层 Extract 函数按 MIME 类型和文件扩展名分派。
//
// 构建说明：PDF 使用 dslipak/pdf（纯 Go）；Excel 使用 xuri/excelize/v2
// （纯 Go）；DOCX/ODT/RTF 使用 lu4p/cat（纯 Go）；PPTX 直接用标准库解析
// ZIP/XML 结构。
package chat

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	pdflib "github.com/dslipak/pdf"
	catlib "github.com/lu4p/cat"
	"github.com/xuri/excelize/v2"
	"golang.org/x/net/html"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
)

// IsImage reports whether the MIME type represents an image that should be
// sent via the LLM Vision path rather than extracted as text.
//
// IsImage 报告 MIME 类型是否为图片——图片走 LLM Vision 路径而非文本提取。
func IsImage(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/")
}

// Extract reads the file at storagePath and returns its text content.
// Images are not handled here — callers should check IsImage first.
//
// Returns:
//   - (text, nil)  on success
//   - ("", ErrAttachmentTypeUnsupported) if no extractor handles the type
//   - ("", ErrAttachmentParseFailed)     if extraction fails
//
// Extract 读取 storagePath 的文件并返回文本内容。
// 图片不在此处处理——调用方应先调 IsImage。
//
// 返回：
//   - (text, nil)  成功
//   - ("", ErrAttachmentTypeUnsupported) 没有提取器支持该类型
//   - ("", ErrAttachmentParseFailed)     提取失败
func Extract(storagePath, mimeType string) (string, error) {
	ext := strings.ToLower(filepath.Ext(storagePath))

	switch {
	case isPlainText(mimeType, ext):
		return extractPlainText(storagePath)
	case mimeType == "application/pdf" || ext == ".pdf":
		return extractPDF(storagePath)
	case isOfficeDoc(mimeType, ext):
		return extractOfficeDoc(storagePath)
	case isExcel(mimeType, ext):
		return extractExcel(storagePath)
	case ext == ".pptx":
		return extractPPTX(storagePath)
	case mimeType == "text/html" || ext == ".html" || ext == ".htm":
		return extractHTML(storagePath)
	default:
		return "", chatdomain.ErrAttachmentTypeUnsupported
	}
}

// ── Plain text ────────────────────────────────────────────────────────────────

func isPlainText(mimeType, ext string) bool {
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}
	switch ext {
	case ".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".rs",
		".rb", ".php", ".sh", ".yaml", ".yml", ".toml", ".ini", ".env",
		".json", ".csv", ".tsv", ".md", ".markdown", ".rst", ".xml", ".sql":
		return true
	}
	return false
}

func extractPlainText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	return string(b), nil
}

// ── PDF ───────────────────────────────────────────────────────────────────────

func extractPDF(path string) (string, error) {
	r, err := pdflib.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, reader); err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	return buf.String(), nil
}

// ── Office docs (DOCX / ODT / RTF) ───────────────────────────────────────────

func isOfficeDoc(mimeType, ext string) bool {
	switch ext {
	case ".docx", ".odt", ".rtf":
		return true
	}
	switch mimeType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.oasis.opendocument.text",
		"application/rtf", "text/rtf":
		return true
	}
	return false
}

func extractOfficeDoc(path string) (string, error) {
	text, err := catlib.File(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	return text, nil
}

// ── Excel ─────────────────────────────────────────────────────────────────────

func isExcel(mimeType, ext string) bool {
	switch ext {
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return true
	}
	return mimeType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}

func extractExcel(path string) (string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	defer f.Close()

	var sb strings.Builder
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		sb.WriteString("[Sheet: ")
		sb.WriteString(sheet)
		sb.WriteString("]\n")
		for _, row := range rows {
			sb.WriteString(strings.Join(row, "\t"))
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// ── PPTX ──────────────────────────────────────────────────────────────────────

// extractPPTX parses a PPTX file (ZIP archive) and collects all <a:t> text
// elements from each slide's XML. No external dependency — stdlib only.
//
// extractPPTX 解析 PPTX 文件（ZIP 压缩包），从每张幻灯片的 XML 中提取所有
// <a:t> 文本元素。纯标准库，无外部依赖。
func extractPPTX(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	defer r.Close()

	var sb strings.Builder
	for _, f := range r.File {
		if !strings.HasPrefix(f.Name, "ppt/slides/slide") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		texts, _ := pptxSlideText(rc)
		rc.Close()
		sb.WriteString(texts)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}

// pptxSlideText collects the content of every <a:t> element in one slide XML.
//
// pptxSlideText 收集一张幻灯片 XML 里所有 <a:t> 元素的内容。
func pptxSlideText(r io.Reader) (string, error) {
	var sb strings.Builder
	dec := xml.NewDecoder(r)
	inText := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			inText = t.Name.Local == "t"
		case xml.CharData:
			if inText {
				sb.Write(t)
				sb.WriteByte(' ')
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
		}
	}
	return sb.String(), nil
}

// ── HTML ──────────────────────────────────────────────────────────────────────

// extractHTML strips HTML tags and returns the visible text content.
//
// extractHTML 剥离 HTML 标签，返回可见的文本内容。
func extractHTML(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	defer f.Close()

	doc, err := html.Parse(f)
	if err != nil {
		return "", fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}

	var sb strings.Builder
	htmlText(doc, &sb)
	return sb.String(), nil
}

// htmlText recursively walks the HTML node tree collecting text nodes,
// skipping script and style elements.
//
// htmlText 递归遍历 HTML 节点树，收集文本节点，跳过 script 和 style。
func htmlText(n *html.Node, sb *strings.Builder) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style":
			return
		}
	}
	if n.Type == html.TextNode {
		trimmed := strings.TrimSpace(n.Data)
		if trimmed != "" {
			sb.WriteString(trimmed)
			sb.WriteByte('\n')
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		htmlText(c, sb)
	}
}
