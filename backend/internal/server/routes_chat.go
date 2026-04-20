package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sunweilin/forgify/internal/attachment"
)

type chatAttachment struct {
	Name   string `json:"name"`
	Base64 string `json:"base64"`
	Size   int64  `json:"size"`
}

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConversationID string           `json:"conversationId"`
		Message        string           `json:"message"`
		Attachments    []chatAttachment `json:"attachments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.ConversationID == "" || req.Message == "" {
		jsonError(w, "conversationId and message are required", http.StatusBadRequest)
		return
	}

	if len(req.Attachments) > attachment.MaxFiles {
		jsonError(w, fmt.Sprintf("最多同时附加 %d 个文件", attachment.MaxFiles), http.StatusBadRequest)
		return
	}

	// Parse attachments
	var files []*attachment.FileInfo
	for _, a := range req.Attachments {
		kind, err := attachment.Validate(a.Name, a.Size)
		if err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		data, err := base64.StdEncoding.DecodeString(a.Base64)
		if err != nil {
			jsonError(w, "文件数据格式错误", http.StatusBadRequest)
			return
		}
		files = append(files, &attachment.FileInfo{
			Name:    a.Name,
			Size:    a.Size,
			Kind:    kind,
			Content: data,
		})
	}

	if err := s.chatSvc.SendMessageWithAttachments(r.Context(), req.ConversationID, req.Message, files); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) stopGeneration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConversationID string `json:"conversationId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	s.chatSvc.StopGeneration(req.ConversationID)
	w.WriteHeader(http.StatusNoContent)
}
