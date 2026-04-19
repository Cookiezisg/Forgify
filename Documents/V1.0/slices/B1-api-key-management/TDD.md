# B1 · API Key 管理 — 技术设计文档

**切片**：B1  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| Key 加密 | AES-256-GCM + 机器指纹作为密钥 | 纯 Go 标准库，Key 不离开本机 |
| 机器指纹 | macOS: IOPlatformSerialNumber；Windows: MachineGuid；Linux: /etc/machine-id | OS 原生，稳定唯一 |
| 存储位置 | SQLite `api_keys` 表 | 与其他数据统一管理 |
| 测试连接 | 发送一个最小的 chat completion 请求 | 最真实的验证方式 |

---

## 2. 目录结构

```
internal/
├── crypto/
│   ├── encrypt.go       # AES-256-GCM 加解密
│   └── fingerprint.go   # 机器指纹获取
└── service/
    └── apikey.go        # APIKey CRUD + 测试连接

internal/storage/migrations/
└── 002_api_keys.sql

frontend/src/
├── pages/
│   └── Settings.tsx         # 设置页框架
└── components/settings/
    ├── ApiKeyList.tsx        # 提供商卡片列表
    └── ApiKeyDrawer.tsx      # 配置抽屉
```

---

## 3. 数据库迁移

### 002_api_keys.sql

```sql
CREATE TABLE IF NOT EXISTS api_keys (
    id           TEXT PRIMARY KEY,
    provider     TEXT NOT NULL,       -- 'anthropic' | 'openai' | 'openai_compat' | ...
    display_name TEXT,                -- 用户自定义名称（openai_compat 多个实例时用）
    key_enc      TEXT NOT NULL,       -- AES-256-GCM 加密后的 Base64
    base_url     TEXT,                -- OpenAI 兼容端点用
    last_tested  DATETIME,
    test_status  TEXT,                -- 'ok' | 'error' | null
    created_at   DATETIME DEFAULT (datetime('now')),
    updated_at   DATETIME DEFAULT (datetime('now'))
);
```

---

## 4. 加密实现

### `internal/crypto/encrypt.go`

```go
package crypto

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "io"
)

// DeriveKey 从机器指纹派生 32 字节 AES Key
func DeriveKey(fingerprint string) []byte {
    h := sha256.Sum256([]byte("forgify-v1:" + fingerprint))
    return h[:]
}

func Encrypt(plaintext, key []byte) (string, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }
    gcm, _ := cipher.NewGCM(block)
    nonce := make([]byte, gcm.NonceSize())
    io.ReadFull(rand.Reader, nonce)
    ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(encoded string, key []byte) ([]byte, error) {
    data, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil {
        return nil, err
    }
    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)
    nonceSize := gcm.NonceSize()
    return gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}
```

### `internal/crypto/fingerprint.go`

```go
package crypto

import (
    "os/exec"
    "runtime"
    "strings"
    "os"
)

func MachineFingerprint() string {
    switch runtime.GOOS {
    case "darwin":
        out, _ := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
        // 解析 IOPlatformSerialNumber
        for _, line := range strings.Split(string(out), "\n") {
            if strings.Contains(line, "IOPlatformSerialNumber") {
                parts := strings.Split(line, "\"")
                if len(parts) >= 4 {
                    return parts[3]
                }
            }
        }
    case "windows":
        out, _ := exec.Command("reg", "query",
            `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
        parts := strings.Fields(string(out))
        if len(parts) > 0 {
            return parts[len(parts)-1]
        }
    default:
        data, _ := os.ReadFile("/etc/machine-id")
        return strings.TrimSpace(string(data))
    }
    return "forgify-fallback-key"
}
```

---

## 5. 业务层

### `internal/service/apikey.go`

```go
package service

import (
    "context"
    "database/sql"
    "forgify/internal/crypto"
    "forgify/internal/storage"
    "time"

    "github.com/google/uuid"
)

type APIKey struct {
    ID          string
    Provider    string
    DisplayName string
    KeyMasked   string // 只返回给前端，格式: sk-...****abcd
    BaseURL     string
    TestStatus  string
    LastTested  *time.Time
}

var encKey []byte

func init() {
    encKey = crypto.DeriveKey(crypto.MachineFingerprint())
}

func SaveAPIKey(provider, displayName, rawKey, baseURL string) (*APIKey, error) {
    enc, err := crypto.Encrypt([]byte(rawKey), encKey)
    if err != nil {
        return nil, err
    }
    id := uuid.NewString()
    _, err = storage.DB().Exec(`
        INSERT INTO api_keys (id, provider, display_name, key_enc, base_url)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET key_enc=excluded.key_enc, base_url=excluded.base_url, updated_at=datetime('now')
    `, id, provider, displayName, enc, baseURL)
    if err != nil {
        return nil, err
    }
    return &APIKey{
        ID: id, Provider: provider,
        KeyMasked: maskKey(rawKey), BaseURL: baseURL,
    }, nil
}

func GetRawKey(id string) (string, error) {
    var enc string
    storage.DB().QueryRow("SELECT key_enc FROM api_keys WHERE id=?", id).Scan(&enc)
    raw, err := crypto.Decrypt(enc, encKey)
    return string(raw), err
}

func maskKey(key string) string {
    if len(key) <= 8 {
        return "****"
    }
    prefix := key[:7]  // 如 "sk-ant-"
    suffix := key[len(key)-4:]
    return prefix + "...****" + suffix
}

// TestConnection 用最小请求验证 Key 是否有效
func TestConnection(ctx context.Context, provider, rawKey, baseURL string) ([]string, error) {
    // 根据 provider 构造 Eino ChatModel，发一个 "hi" 请求
    // 返回可用模型列表或错误
    // 具体实现在 B2 完成后补充
    return nil, nil
}
```

---

## 6. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/api-keys", s.listAPIKeys)
mux.HandleFunc("POST /api/api-keys", s.saveAPIKey)
mux.HandleFunc("DELETE /api/api-keys/{id}", s.deleteAPIKey)
mux.HandleFunc("POST /api/api-keys/test", s.testAPIKey)
```

---

## 7. 前端组件结构

```tsx
// ApiKeyList.tsx — 提供商卡片列表
// 从 ListAPIKeys() 获取数据，渲染每个提供商的状态卡片

// ApiKeyDrawer.tsx — 配置抽屉
// 包含：Key 输入框（type=password）、Base URL 输入框（可选）
// "测试连接"调用 TestAPIKey()，实时显示结果
// "保存"调用 SaveAPIKey()
```

---

## 8. 验收测试

```
1. 配置 Anthropic Key → 保存 → 重启 → Key 仍存在（显示 masked）
2. 正确 Key 测试连接 → 绿色✓ + 模型列表
3. 错误 Key 测试连接 → 红色✗ + 错误描述
4. 删除 Key → 卡片回到"未配置"
5. 无任何 Key 时，对话页显示引导提示
6. 查看 SQLite 内容：key_enc 字段是加密的，不是明文
```
