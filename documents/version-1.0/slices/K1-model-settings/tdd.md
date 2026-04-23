# K1 · 模型设置 — 技术设计文档

**切片**：K1  
**状态**：待 Review

---

## 1. 数据库

```sql
-- 012_settings.sql
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

模型配置存储为 JSON，key 为 `model_config`：

```json
{
  "chat_model": { "provider": "anthropic", "model": "claude-3-5-sonnet", "api_key": "sk-..." },
  "llm_node_model": { "provider": "openai", "model": "gpt-4o", "api_key": "sk-..." },
  "agent_node_model": { "provider": "anthropic", "model": "claude-3-opus", "api_key": "sk-..." },
  "ollama_base_url": "http://localhost:11434"
}
```

API Key 与模型配置一起存储（本地 SQLite，不上传）。

---

## 2. SettingsService

```go
// service/settings.go
type ModelConfig struct {
    Provider string `json:"provider"` // anthropic, openai, ollama
    Model    string `json:"model"`
    APIKey   string `json:"apiKey,omitempty"`
}

type ModelSettings struct {
    ChatModel      ModelConfig `json:"chat_model"`
    LLMNodeModel   ModelConfig `json:"llm_node_model"`
    AgentNodeModel ModelConfig `json:"agent_node_model"`
    OllamaBaseURL  string      `json:"ollama_base_url"`
}

type SettingsService struct{}

func (s *SettingsService) GetModelSettings() (*ModelSettings, error) {
    var raw string
    err := storage.DB().QueryRow(
        `SELECT value FROM settings WHERE key='model_config'`).Scan(&raw)
    if err == sql.ErrNoRows {
        return s.defaultModelSettings(), nil
    }
    var ms ModelSettings
    json.Unmarshal([]byte(raw), &ms)
    return &ms, nil
}

func (s *SettingsService) SaveModelSettings(ms *ModelSettings) error {
    raw, _ := json.Marshal(ms)
    _, err := storage.DB().Exec(`
        INSERT INTO settings (key, value) VALUES ('model_config', ?)
        ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
    return err
}

func (s *SettingsService) defaultModelSettings() *ModelSettings {
    return &ModelSettings{
        ChatModel:      ModelConfig{Provider: "anthropic", Model: "claude-3-5-sonnet"},
        LLMNodeModel:   ModelConfig{Provider: "anthropic", Model: "claude-3-5-sonnet"},
        AgentNodeModel: ModelConfig{Provider: "anthropic", Model: "claude-3-opus"},
        OllamaBaseURL:  "http://localhost:11434",
    }
}
```

---

## 3. 连接测试

```go
// service/settings.go
func (s *SettingsService) TestModelConnection(config ModelConfig) (bool, string, error) {
    var llm eino.ChatModel
    switch config.Provider {
    case "anthropic":
        llm = anthropic.NewChatModel(config.APIKey, config.Model)
    case "openai":
        llm = openai.NewChatModel(config.APIKey, config.Model)
    case "ollama":
        llm = ollama.NewChatModel(s.getOllamaURL(), config.Model)
    default:
        return false, "", fmt.Errorf("未知提供商: %s", config.Provider)
    }

    start := time.Now()
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    _, err := llm.Generate(ctx, []*schema.Message{
        {Role: "user", Content: "ping"},
    })
    if err != nil {
        return false, "", err
    }

    latency := time.Since(start).Milliseconds()
    return true, fmt.Sprintf("连接正常 · %dms", latency), nil
}
```

---

## 4. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/settings/model", s.getModelSettings)
mux.HandleFunc("POST /api/settings/model", s.saveModelSettings)
mux.HandleFunc("POST /api/settings/model/test", s.testModelConnection)
```

---

## 5. 前端：模型设置页

```tsx
// pages/settings/ModelSettingsPage.tsx
export function ModelSettingsPage() {
    const [settings, setSettings] = useState<ModelSettings | null>(null)
    const [testResults, setTestResults] = useState<Record<string, string>>({})

    useEffect(() => {
        fetch(`http://127.0.0.1:${port}/api/settings/model`).then(r => r.json()).then(setSettings)
    }, [])

    const handleTest = async (key: string, config: ModelConfig) => {
        const res = await fetch(`http://127.0.0.1:${port}/api/settings/model/test`, {
            method: 'POST', headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(config),
        }).then(r => r.json())
        setTestResults(r => ({ ...r, [key]: res.ok ? `✓ ${res.message}` : `✗ 连接失败` }))
    }

    if (!settings) return null

    return (
        <div className="p-6 space-y-6 max-w-xl">
            <h2 className="text-base font-semibold">模型设置</h2>

            {(['chat_model', 'llm_node_model', 'agent_node_model'] as const).map(key => (
                <ModelConfigSection key={key}
                    label={{ chat_model: '对话模型', llm_node_model: 'LLM 节点模型', agent_node_model: 'Agent 节点模型' }[key]}
                    config={settings[key]}
                    onChange={cfg => setSettings(s => s ? { ...s, [key]: cfg } : s)}
                    testResult={testResults[key]}
                    onTest={() => handleTest(key, settings[key])}
                />
            ))}

            <button onClick={() => SaveModelSettings(settings!).then(() => alert('已保存'))}
                className="px-4 py-2 bg-blue-600 rounded text-sm">保存</button>
        </div>
    )
}

function ModelConfigSection({ label, config, onChange, testResult, onTest }) {
    return (
        <div className="space-y-3">
            <p className="text-sm font-medium">{label}</p>
            <select value={config.provider} onChange={e => onChange({ ...config, provider: e.target.value })}
                className="w-full px-3 py-2 bg-neutral-800 rounded text-sm">
                <option value="anthropic">Anthropic</option>
                <option value="openai">OpenAI</option>
                <option value="ollama">Ollama（本地）</option>
            </select>
            <input placeholder="模型名" value={config.model}
                onChange={e => onChange({ ...config, model: e.target.value })}
                className="w-full px-3 py-2 bg-neutral-800 rounded text-sm" />
            {config.provider !== 'ollama' && (
                <input type="password" placeholder="API Key"
                    value={config.apiKey ?? ''}
                    onChange={e => onChange({ ...config, apiKey: e.target.value })}
                    className="w-full px-3 py-2 bg-neutral-800 rounded text-sm font-mono" />
            )}
            <div className="flex items-center gap-3">
                <button onClick={onTest} className="text-xs px-3 py-1 rounded bg-neutral-700">测试连接</button>
                {testResult && <span className={`text-xs ${testResult.startsWith('✓') ? 'text-green-400' : 'text-red-400'}`}>{testResult}</span>}
            </div>
        </div>
    )
}
```

---

## 6. 验收测试

```
1. GetModelSettings() 返回默认配置（首次启动）
2. SaveModelSettings() → DB 持久化 → 重启后仍然有效
3. TestModelConnection() with 合法 API Key → 返回 true + 延迟
4. TestModelConnection() with 错误 Key → 返回 false + 错误信息
5. Ollama provider → 不显示 API Key 输入框
6. 保存后 chatSvc.RefreshLLMClients() 被调用，新对话使用新模型
```
