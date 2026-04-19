# G1 · Cron 定时调度 — 技术设计文档

**切片**：G1  
**状态**：待 Review

---

## 1. 依赖

```
github.com/robfig/cron/v3  # Go cron 库，5字段标准格式
```

---

## 2. 目录结构

```
internal/
└── scheduler/
    └── scheduler.go   # CronScheduler
```

---

## 3. CronScheduler

```go
// scheduler/scheduler.go
package scheduler

import (
    "encoding/json"
    "log"

    "github.com/robfig/cron/v3"
    "forgify/internal/runner"
    "forgify/internal/service"
)

type CronScheduler struct {
    c           *cron.Cron
    workflowSvc *service.WorkflowService
    runner      *runner.Runner
    entryIDs    map[string]cron.EntryID // workflowID → cron entry ID
}

func New(workflowSvc *service.WorkflowService, r *runner.Runner) *CronScheduler {
    return &CronScheduler{
        c:           cron.New(),
        workflowSvc: workflowSvc,
        runner:      r,
        entryIDs:    map[string]cron.EntryID{},
    }
}

// Start 启动调度器，扫描已部署工作流并注册定时任务
func (s *CronScheduler) Start() {
    wfs, _ := s.workflowSvc.ListDeployed()
    for _, wf := range wfs {
        s.Register(wf)
    }
    s.c.Start()
}

// Register 为工作流注册定时任务
func (s *CronScheduler) Register(wf *service.Workflow) {
    cronExpr := s.extractCron(wf.Definition)
    if cronExpr == "" { return }

    id, err := s.c.AddFunc(cronExpr, func() {
        log.Printf("cron: triggering workflow %s", wf.ID)
        s.runner.Run(nil, wf.ID, nil)
    })
    if err != nil {
        log.Printf("cron: invalid expression for workflow %s: %v", wf.ID, err)
        return
    }
    s.entryIDs[wf.ID] = id
}

// Deregister 取消工作流的定时任务
func (s *CronScheduler) Deregister(workflowID string) {
    if id, ok := s.entryIDs[workflowID]; ok {
        s.c.Remove(id)
        delete(s.entryIDs, workflowID)
    }
}

func (s *CronScheduler) extractCron(def json.RawMessage) string {
    var fd struct {
        Nodes []struct {
            Type   string         `json:"type"`
            Config map[string]any `json:"config"`
        } `json:"nodes"`
    }
    json.Unmarshal(def, &fd)
    for _, n := range fd.Nodes {
        if n.Type == "trigger_schedule" {
            if cron, ok := n.Config["cron"].(string); ok {
                return cron
            }
        }
    }
    return ""
}
```

---

## 4. WorkflowService 扩展

```go
// service/workflow.go
func (s *WorkflowService) ListDeployed() ([]*Workflow, error) {
    rows, _ := storage.DB().Query(
        `SELECT id, name, definition, status, created_at, updated_at
         FROM workflows WHERE status = 'deployed'`)
    // ... scan ...
}
```

---

## 5. 与部署/暂停联动

```go
// backend/internal/server/routes.go（通过 G2 路由的 deploy/pause handler 触发）
// scheduler.Register(wf) 在 deployWorkflow handler 中调用
// scheduler.Deregister(id) 在 pauseWorkflow handler 中调用
```

---

## 6. 无效 Cron 表达式验证

在部署前（或编辑节点配置时）校验：

```go
// compiler/validator.go 中增加
import "github.com/robfig/cron/v3"

func validateCronExpr(expr string) error {
    parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
    _, err := parser.Parse(expr)
    return err
}
```

---

## 7. 验收测试

```
1. Start() → 扫描 deployed 工作流 → 含 trigger_schedule 的被注册
2. cron="* * * * *" → 每分钟调用 runner.Run()
3. Deregister() → 定时任务停止
4. 无效 cron 表达式 → Register() 打印错误，不注册
5. DeployWorkflow() → 自动注册定时任务
6. PauseWorkflow() → 自动取消定时任务
```
