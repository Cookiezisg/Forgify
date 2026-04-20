package model

import (
	"github.com/sunweilin/forgify/internal/storage"
)

type ModelPurpose string

const (
	PurposeConversation ModelPurpose = "conversation"
	PurposeCodegen      ModelPurpose = "codegen"
	PurposeCheap        ModelPurpose = "cheap"
)

type ModelAssignment struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

func (a ModelAssignment) IsEmpty() bool {
	return a.Provider == "" || a.ModelID == ""
}

type ModelConfig struct {
	Conversation ModelAssignment `json:"conversation"`
	Codegen      ModelAssignment `json:"codegen"`
	Cheap        ModelAssignment `json:"cheap"`
	Fallback     ModelAssignment `json:"fallback"`
}

func (c *ModelConfig) ForPurpose(p ModelPurpose) ModelAssignment {
	switch p {
	case PurposeCodegen:
		if !c.Codegen.IsEmpty() {
			return c.Codegen
		}
		return c.Conversation
	case PurposeCheap:
		if !c.Cheap.IsEmpty() {
			return c.Cheap
		}
		return c.Conversation
	default:
		return c.Conversation
	}
}

func LoadModelConfig() (*ModelConfig, error) {
	get := func(key string) string {
		var v string
		storage.DB().QueryRow(`SELECT value FROM app_config WHERE key=?`, key).Scan(&v)
		return v
	}
	parse := func(raw string) ModelAssignment {
		if len(raw) < 3 {
			return ModelAssignment{}
		}
		for i, c := range raw {
			if c == ':' {
				return ModelAssignment{Provider: raw[:i], ModelID: raw[i+1:]}
			}
		}
		return ModelAssignment{}
	}
	return &ModelConfig{
		Conversation: parse(get("model.conversation")),
		Codegen:      parse(get("model.codegen")),
		Cheap:        parse(get("model.cheap")),
		Fallback:     parse(get("model.fallback")),
	}, nil
}

func SaveModelConfig(cfg *ModelConfig) error {
	encode := func(a ModelAssignment) string {
		if a.IsEmpty() {
			return ""
		}
		return a.Provider + ":" + a.ModelID
	}
	pairs := map[string]string{
		"model.conversation": encode(cfg.Conversation),
		"model.codegen":      encode(cfg.Codegen),
		"model.cheap":        encode(cfg.Cheap),
		"model.fallback":     encode(cfg.Fallback),
	}
	for k, v := range pairs {
		if _, err := storage.DB().Exec(`
			INSERT INTO app_config(key,value) VALUES(?,?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=datetime('now')
		`, k, v); err != nil {
			return err
		}
	}
	return nil
}
