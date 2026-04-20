package attachment

import (
	"fmt"
	"path/filepath"
	"strings"
)

const MaxFileSize = 20 * 1024 * 1024 // 20 MB
const MaxFiles = 5

var supportedExts = map[string]string{
	// Text / code
	".txt": "text", ".md": "text", ".csv": "text",
	".json": "text", ".yaml": "text", ".yml": "text",
	".xml": "text", ".log": "text", ".toml": "text",
	".py": "text", ".js": "text", ".ts": "text", ".tsx": "text", ".jsx": "text",
	".go": "text", ".sql": "text", ".sh": "text", ".bash": "text",
	".java": "text", ".rs": "text", ".c": "text", ".cpp": "text", ".h": "text",
	".html": "text", ".css": "text", ".scss": "text",
	// Office
	".xlsx": "excel", ".xls": "excel",
	".pdf": "pdf",
	".docx": "word",
	// Image
	".png": "image", ".jpg": "image", ".jpeg": "image",
	".gif": "image", ".webp": "image",
}

// FileInfo holds an uploaded file's metadata and raw bytes.
type FileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Kind    string `json:"kind"` // "text"|"excel"|"pdf"|"word"|"image"
	Content []byte `json:"-"`
}

// Validate checks file type and size. Returns an error with a user-friendly message.
func Validate(name string, size int64) (string, error) {
	if size > MaxFileSize {
		return "", fmt.Errorf("文件超过 20MB 限制")
	}
	ext := strings.ToLower(filepath.Ext(name))
	kind, ok := supportedExts[ext]
	if !ok {
		return "", fmt.Errorf("不支持 %s 文件，请使用文本或 Office 文件", ext)
	}
	return kind, nil
}

// MIMEType returns the MIME type for an image file name.
func MIMEType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	m := map[string]string{
		".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".gif": "image/gif", ".webp": "image/webp",
	}
	if v, ok := m[ext]; ok {
		return v
	}
	return "application/octet-stream"
}
