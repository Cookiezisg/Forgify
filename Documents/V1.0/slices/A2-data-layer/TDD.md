# A2 · 数据层基础 — 技术设计文档

**切片**：A2  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| SQLite 驱动 | `modernc.org/sqlite` | 纯 Go，零 CGO，支持交叉编译 |
| 向量检索 | `viant/sqlite-vec` | 纯 Go 封装，主题记忆语义检索（I 系列切片用） |
| 迁移框架 | 自定义（编号 SQL 文件） | 依赖少，逻辑透明，够用 |
| WAL 模式 | 开启 | 并发读写性能更好，适合后台任务和 UI 同时访问 |
| 连接模式 | 单连接 + mutex | 避免 SQLite 并发写冲突，够简单 |

---

## 2. 目录结构

```
internal/storage/
├── db.go                  # 连接管理、初始化、迁移入口
├── migrations/
│   ├── 001_init.sql       # 初始 schema（本切片）
│   └── ...                # 后续切片各自添加
└── queries/               # 各模块的 SQL 查询（后续切片填充）
```

---

## 3. 初始 Schema（001_init.sql）

本切片只建基础表，业务表随各切片迁移文件添加。

```sql
-- schema 版本追踪
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  DATETIME DEFAULT (datetime('now'))
);

-- 应用配置（key-value）
CREATE TABLE IF NOT EXISTS app_config (
    key         TEXT PRIMARY KEY,
    value       TEXT,
    updated_at  DATETIME DEFAULT (datetime('now'))
);
```

后续切片（如 B1）会添加 `002_api_keys.sql`、`003_conversations.sql` 等。

---

## 4. 核心实现

### 4.1 db.go

```go
package storage

import (
    "database/sql"
    "embed"
    "fmt"
    "os"
    "path/filepath"
    "sync"

    _ "modernc.org/sqlite"
    vec "github.com/viant/sqlite-vec"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var (
    db   *sql.DB
    once sync.Once
)

func Init(dataDir string) error {
    var initErr error
    once.Do(func() {
        if err := os.MkdirAll(dataDir, 0755); err != nil {
            initErr = err
            return
        }
        dbPath := filepath.Join(dataDir, "forgify.db")
        conn, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
        if err != nil {
            initErr = err
            return
        }
        // 注册 sqlite-vec 扩展
        if err := vec.Register(conn); err != nil {
            initErr = err
            return
        }
        conn.SetMaxOpenConns(1) // SQLite 单写连接
        db = conn
        initErr = migrate(conn)
    })
    return initErr
}

func DB() *sql.DB { return db }
```

### 4.2 迁移逻辑

```go
func migrate(db *sql.DB) error {
    // 读取所有迁移文件，按编号排序
    entries, _ := migrationFiles.ReadDir("migrations")
    for _, e := range entries {
        var version int
        fmt.Sscanf(e.Name(), "%d_", &version)

        // 检查是否已应用
        var count int
        db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=?", version).Scan(&count)
        if count > 0 {
            continue
        }

        // 执行迁移
        sql, _ := migrationFiles.ReadFile("migrations/" + e.Name())
        if _, err := db.Exec(string(sql)); err != nil {
            return fmt.Errorf("migration %d failed: %w", version, err)
        }
        db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version)
    }
    return nil
}
```

### 4.3 数据目录解析

```go
// internal/storage/datadir.go
package storage

import (
    "os"
    "path/filepath"
    "runtime"
)

func DefaultDataDir() string {
    switch runtime.GOOS {
    case "darwin":
        home, _ := os.UserHomeDir()
        return filepath.Join(home, "Library", "Application Support", "Forgify")
    case "windows":
        return filepath.Join(os.Getenv("APPDATA"), "Forgify")
    default:
        home, _ := os.UserHomeDir()
        return filepath.Join(home, ".config", "Forgify")
    }
}

// 从 app_config 读取用户自定义路径（如果有）
func DataDir() string {
    if db == nil {
        return DefaultDataDir()
    }
    var custom string
    db.QueryRow("SELECT value FROM app_config WHERE key='data_dir'").Scan(&custom)
    if custom != "" {
        return custom
    }
    return DefaultDataDir()
}
```

---

## 5. 在 main.go 中初始化

```go
func main() {
    dataDir := storage.DefaultDataDir()
    if err := storage.Init(dataDir); err != nil {
        // 迁移失败：通过 Wails dialog 告知用户
        dialog.Error(fmt.Sprintf("数据库初始化失败：%v", err))
        return
    }
    // ... 启动 Wails app
}
```

---

## 6. 验收测试

```
1. 首次运行：dataDir 目录自动创建，forgify.db 存在
2. 重复运行：schema_migrations 不重复插入
3. 新增迁移文件 002_test.sql，重启后自动执行
4. 删除 forgify.db，重新运行：数据库重新创建
5. 模拟 migrate() 返回错误：main.go 捕获并处理
6. DB() 在 Init 前调用返回 nil（调用方需判断）
```
