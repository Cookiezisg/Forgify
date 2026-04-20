package server

import (
	"io"
	"net/http"

	"github.com/sunweilin/forgify/internal/attachment"
)

// handleUploadAttachment validates and extracts text from an uploaded file.
// Returns JSON with the file info and extracted content preview.
func (s *Server) handleUploadAttachment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(attachment.MaxFileSize + 1024); err != nil {
		jsonError(w, "请求过大", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "未找到上传文件", http.StatusBadRequest)
		return
	}
	defer file.Close()

	kind, err := attachment.Validate(header.Filename, header.Size)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		jsonError(w, "读取文件失败", http.StatusInternalServerError)
		return
	}

	info := &attachment.FileInfo{
		Name:    header.Filename,
		Size:    header.Size,
		Kind:    kind,
		Content: data,
	}

	// For non-image files, extract text preview
	var preview string
	if kind != "image" {
		text, err := attachment.Extract(info)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Truncate preview for the response
		runes := []rune(text)
		if len(runes) > 500 {
			preview = string(runes[:500]) + "..."
		} else {
			preview = text
		}
	}

	// Store in the session cache for later use during chat send.
	// For MVP, files are sent as base64 in the chat request body instead.
	jsonOK(w, map[string]any{
		"name":    info.Name,
		"size":    info.Size,
		"kind":    info.Kind,
		"preview": preview,
	})
}
