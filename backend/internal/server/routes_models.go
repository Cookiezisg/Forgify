package server

import (
	"encoding/json"
	"net/http"

	"github.com/sunweilin/forgify/internal/model"
	"github.com/sunweilin/forgify/internal/service"
)

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	keys, err := service.ListAPIKeys()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	seen := make(map[string]bool)
	var providers []string
	for _, k := range keys {
		if !seen[k.Provider] {
			seen[k.Provider] = true
			providers = append(providers, k.Provider)
		}
	}
	models := model.AvailableModels(providers)
	if models == nil {
		models = []model.ModelInfo{}
	}
	jsonOK(w, models)
}

func (s *Server) getModelConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := model.LoadModelConfig()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, cfg)
}

func (s *Server) saveModelConfig(w http.ResponseWriter, r *http.Request) {
	var cfg model.ModelConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := model.SaveModelConfig(&cfg); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
