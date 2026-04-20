package server

import (
	"encoding/json"
	"net/http"

	"github.com/sunweilin/forgify/internal/model"
	"github.com/sunweilin/forgify/internal/service"
)

// autoConfigureModelIfFirst sets up the default model config when the first API key is saved.
// Picks the best available model from the provider as the conversation model.
func autoConfigureModelIfFirst(provider string) {
	cfg, _ := model.LoadModelConfig()
	if cfg == nil || !cfg.Conversation.IsEmpty() {
		return // Already configured
	}

	models, ok := model.ProviderModels[provider]
	if !ok || len(models) == 0 {
		return
	}

	// Pick best model: prefer "balanced", then "powerful", then first available
	var pick model.ModelInfo
	for _, m := range models {
		if m.Tier == "balanced" {
			pick = m
			break
		}
	}
	if pick.ID == "" {
		for _, m := range models {
			if m.Tier == "powerful" {
				pick = m
				break
			}
		}
	}
	if pick.ID == "" {
		pick = models[0]
	}

	model.SaveModelConfig(&model.ModelConfig{
		Conversation: model.ModelAssignment{Provider: provider, ModelID: pick.ID},
	})
}

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := service.ListAPIKeys()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if keys == nil {
		keys = []service.APIKey{}
	}
	jsonOK(w, keys)
}

func (s *Server) saveAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string `json:"id"`
		Provider    string `json:"provider"`
		DisplayName string `json:"displayName"`
		Key         string `json:"key"`
		BaseURL     string `json:"baseUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Provider == "" {
		jsonError(w, "provider is required", http.StatusBadRequest)
		return
	}

	// If updating an existing key and no new key provided, only update baseUrl
	if req.ID != "" && req.Key == "" {
		k, err := service.UpdateAPIKeyBaseURL(req.ID, req.BaseURL)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, k)
		return
	}

	if req.Key == "" {
		jsonError(w, "key is required for new API key", http.StatusBadRequest)
		return
	}
	k, err := service.SaveAPIKey(req.ID, req.Provider, req.DisplayName, req.Key, req.BaseURL)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Auto-configure model if this is the first API key and no model is set yet
	autoConfigureModelIfFirst(req.Provider)

	jsonOK(w, k)
}

func (s *Server) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := service.DeleteAPIKey(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Key      string `json:"key"`
		BaseURL  string `json:"baseUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	// If no key provided, try to use the saved key for this provider (re-test saved key)
	testKey := req.Key
	testURL := req.BaseURL
	if testKey == "" {
		savedKey, savedURL, err := service.GetRawKeyForProvider(req.Provider)
		if err != nil {
			jsonOK(w, map[string]any{"ok": false, "message": "没有已保存的 Key"})
			return
		}
		testKey = savedKey
		if testURL == "" {
			testURL = savedURL
		}
	}

	ok, msg, err := service.TestAPIKeyConnection(r.Context(), req.Provider, testKey, testURL)
	if err != nil {
		service.UpdateTestStatusByProvider(req.Provider, "error")
		jsonOK(w, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	if ok {
		service.UpdateTestStatusByProvider(req.Provider, "ok")
	}
	jsonOK(w, map[string]any{"ok": ok, "message": msg})
}
