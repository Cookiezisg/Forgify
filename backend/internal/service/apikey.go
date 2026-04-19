package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sunweilin/forgify/internal/crypto"
	"github.com/sunweilin/forgify/internal/storage"
)

var encKey []byte

func init() {
	encKey = crypto.DeriveKey(crypto.MachineFingerprint())
}

type APIKey struct {
	ID          string     `json:"id"`
	Provider    string     `json:"provider"`
	DisplayName string     `json:"displayName"`
	KeyMasked   string     `json:"keyMasked"`
	BaseURL     string     `json:"baseUrl"`
	TestStatus  string     `json:"testStatus"`
	LastTested  *time.Time `json:"lastTested"`
}

func ListAPIKeys() ([]APIKey, error) {
	rows, err := storage.DB().Query(`
		SELECT id, provider, COALESCE(display_name,''), key_enc, COALESCE(base_url,''),
		       COALESCE(test_status,''), last_tested
		FROM api_keys ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var keyEnc string
		var lastTested *string
		if err := rows.Scan(&k.ID, &k.Provider, &k.DisplayName, &keyEnc, &k.BaseURL, &k.TestStatus, &lastTested); err != nil {
			return nil, err
		}
		raw, _ := crypto.Decrypt(keyEnc, encKey)
		k.KeyMasked = maskKey(string(raw))
		if lastTested != nil {
			t, _ := time.Parse(time.DateTime, *lastTested)
			k.LastTested = &t
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func SaveAPIKey(id, provider, displayName, rawKey, baseURL string) (*APIKey, error) {
	if id == "" {
		id = uuid.NewString()
	}
	enc, err := crypto.Encrypt([]byte(rawKey), encKey)
	if err != nil {
		return nil, err
	}
	_, err = storage.DB().Exec(`
		INSERT INTO api_keys (id, provider, display_name, key_enc, base_url)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			provider=excluded.provider,
			display_name=excluded.display_name,
			key_enc=excluded.key_enc,
			base_url=excluded.base_url,
			updated_at=datetime('now')`,
		id, provider, displayName, enc, baseURL)
	if err != nil {
		return nil, err
	}
	return &APIKey{
		ID: id, Provider: provider,
		DisplayName: displayName,
		KeyMasked:   maskKey(rawKey),
		BaseURL:     baseURL,
	}, nil
}

func DeleteAPIKey(id string) error {
	_, err := storage.DB().Exec(`DELETE FROM api_keys WHERE id=?`, id)
	return err
}

func GetRawKey(id string) (string, error) {
	var enc string
	err := storage.DB().QueryRow(`SELECT key_enc FROM api_keys WHERE id=?`, id).Scan(&enc)
	if err != nil {
		return "", err
	}
	raw, err := crypto.Decrypt(enc, encKey)
	return string(raw), err
}

func GetRawKeyForProvider(provider string) (key, baseURL string, err error) {
	var enc string
	row := storage.DB().QueryRow(`
		SELECT id, key_enc, COALESCE(base_url,'') FROM api_keys
		WHERE provider=? ORDER BY CASE test_status WHEN 'ok' THEN 0 ELSE 1 END, created_at ASC LIMIT 1`,
		provider)
	var id string
	if err = row.Scan(&id, &enc, &baseURL); err != nil {
		return "", "", fmt.Errorf("no API key for provider %q", provider)
	}
	raw, err := crypto.Decrypt(enc, encKey)
	if err != nil {
		return "", "", fmt.Errorf("decrypt key for provider %q: %w", provider, err)
	}
	return string(raw), baseURL, nil
}

func TestAPIKeyConnection(ctx context.Context, provider, rawKey, baseURL string) (bool, string, error) {
	switch provider {
	case "anthropic":
		return testAnthropic(ctx, rawKey)
	case "openai":
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return testOpenAICompat(ctx, rawKey, baseURL)
	case "deepseek":
		if baseURL == "" {
			baseURL = "https://api.deepseek.com/v1"
		}
		return testOpenAICompat(ctx, rawKey, baseURL)
	case "moonshot":
		if baseURL == "" {
			baseURL = "https://api.moonshot.cn/v1"
		}
		return testOpenAICompat(ctx, rawKey, baseURL)
	case "openai_compat":
		return testOpenAICompat(ctx, rawKey, baseURL)
	case "ollama":
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		return testOllama(ctx, baseURL)
	default:
		return false, "", fmt.Errorf("unknown provider: %s", provider)
	}
}

func UpdateTestStatus(id, status string) {
	storage.DB().Exec(`
		UPDATE api_keys SET test_status=?, last_tested=datetime('now'), updated_at=datetime('now')
		WHERE id=?`, status, id)
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	prefix := key[:7]
	suffix := key[len(key)-4:]
	return prefix + "...****" + suffix
}

func testAnthropic(ctx context.Context, apiKey string) (bool, string, error) {
	body := `{"model":"claude-haiku-4-5-20251001","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages",
		bytes.NewBufferString(body))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		return true, "连接成功", nil
	}
	data, _ := io.ReadAll(resp.Body)
	return false, "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
}

func testOpenAICompat(ctx context.Context, apiKey, baseURL string) (bool, string, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	body := `{"model":"gpt-4o-mini","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions",
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		return true, "连接成功", nil
	}
	data, _ := io.ReadAll(resp.Body)
	return false, "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
}

func testOllama(ctx context.Context, baseURL string) (bool, string, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/tags", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var result struct {
		Models []struct{ Name string } `json:"models"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	names := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return true, fmt.Sprintf("连接成功，%d 个模型可用", len(names)), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
