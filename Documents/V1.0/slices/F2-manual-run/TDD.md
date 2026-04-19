# F2 · 手动运行 — 技术设计文档

**切片**：F2  
**状态**：待 Review

---

## 1. 目录结构

```
internal/
├── runner/
│   ├── runner.go        # 主执行引擎
│   ├── context.go       # RunContext：节点间数据传递
│   └── node_runners/    # 每种节点类型的执行逻辑
│       ├── tool.go
│       ├── condition.go
│       ├── llm.go
│       ├── agent.go
│       └── ...
internal/service/
└── run.go               # RunService：管理运行记录

frontend/src/components/workflow/
└── RunParamsDialog.tsx  # 运行参数填写对话框
```

---

## 2. 运行记录（DB）

```sql
-- 008_runs.sql
CREATE TABLE IF NOT EXISTS runs (
    id           TEXT PRIMARY KEY,
    workflow_id  TEXT NOT NULL REFERENCES workflows(id),
    status       TEXT NOT NULL DEFAULT 'running'
                 CHECK(status IN ('running','success','failed','cancelled')),
    trigger_type TEXT NOT NULL DEFAULT 'manual',
    params       JSON,          -- trigger_manual 的入参
    started_at   DATETIME DEFAULT (datetime('now')),
    finished_at  DATETIME,
    error        TEXT
);

CREATE TABLE IF NOT EXISTS run_node_results (
    id         TEXT PRIMARY KEY,
    run_id     TEXT NOT NULL REFERENCES runs(id),
    node_id    TEXT NOT NULL,
    status     TEXT NOT NULL CHECK(status IN ('running','success','failed','skipped')),
    input      JSON,
    output     JSON,
    started_at DATETIME,
    finished_at DATETIME,
    error      TEXT
);

CREATE INDEX IF NOT EXISTS idx_runs_workflow ON runs(workflow_id);
CREATE INDEX IF NOT EXISTS idx_run_nodes_run ON run_node_results(run_id);
```

---

## 3. RunContext

```go
// runner/context.go
type RunContext struct {
    RunID      string
    WorkflowID string
    NodeOutputs map[string]any  // nodeID → output
    mu          sync.RWMutex
}

func (rc *RunContext) SetOutput(nodeID string, output any) {
    rc.mu.Lock()
    defer rc.mu.Unlock()
    rc.NodeOutputs[nodeID] = output
}

func (rc *RunContext) GetOutput(nodeID string) (any, bool) {
    rc.mu.RLock()
    defer rc.mu.RUnlock()
    v, ok := rc.NodeOutputs[nodeID]
    return v, ok
}
```

---

## 4. 主执行引擎

```go
// runner/runner.go
type Runner struct {
    toolSvc    *service.ToolService
    runSvc     *service.RunService
    bridge     *events.Bridge
}

func (r *Runner) Run(ctx context.Context, workflowID string, params map[string]any) (string, error) {
    wf, _ := r.runSvc.GetWorkflow(workflowID)
    result := compiler.Compile(wf.Definition, r.toolSvc)
    if len(result.Errors) > 0 {
        return "", fmt.Errorf("编译失败: %d 个错误", len(result.Errors))
    }

    runID, _ := r.runSvc.Create(workflowID, "manual", params)
    rc := &RunContext{RunID: runID, WorkflowID: workflowID, NodeOutputs: map[string]any{}}

    go r.execute(ctx, runID, result.Graph, rc, wf.Definition)
    return runID, nil
}

func (r *Runner) execute(ctx context.Context, runID string, graph *compose.Graph, rc *RunContext, def json.RawMessage) {
    // 遍历图的拓扑顺序执行节点
    nodes := topologicalSort(def)
    for _, node := range nodes {
        r.emitNodeStatus(runID, node.ID, "running")
        input := compiler.ResolveConfig(node.Config, rc.NodeOutputs)

        output, err := r.runNode(ctx, node, input, rc)
        if err != nil {
            r.runSvc.SetNodeResult(runID, node.ID, "failed", input, nil, err.Error())
            r.emitNodeStatus(runID, node.ID, "failed")
            r.runSvc.Finish(runID, "failed", err.Error())
            r.bridge.Emit(events.RunFailed, map[string]any{"runId": runID, "nodeId": node.ID})
            return
        }

        rc.SetOutput(node.ID, output)
        r.runSvc.SetNodeResult(runID, node.ID, "success", input, output, "")
        r.emitNodeStatus(runID, node.ID, "success")
    }

    r.runSvc.Finish(runID, "success", "")
    r.bridge.Emit(events.RunCompleted, map[string]any{"runId": runID})
}

func (r *Runner) emitNodeStatus(runID, nodeID, status string) {
    r.bridge.Emit(events.NodeStatusChanged, map[string]any{
        "runId": runID, "nodeId": nodeID, "status": status,
    })
}
```

---

## 5. 前端：画布节点状态订阅

```tsx
// WorkflowCanvas.tsx 中增加运行状态监听
const [nodeRunStatus, setNodeRunStatus] = useState<Record<string, string>>({})

useEffect(() => {
    const off1 = onEvent(EV.NodeStatusChanged, ({ nodeId, status }) => {
        setNodeRunStatus(prev => ({ ...prev, [nodeId]: status }))
    })
    const off2 = onEvent(EV.RunCompleted, () => { /* 顶部提示成功 */ })
    const off3 = onEvent(EV.RunFailed, ({ nodeId }) => { /* 顶部提示失败 */ })
    return () => { off1(); off2(); off3() }
}, [])

// 将 runStatus 注入节点 data
const nodesWithStatus = nodes.map(n => ({
    ...n,
    data: { ...n.data, runStatus: nodeRunStatus[n.id] }
}))
```

---

## 6. 运行参数对话框

```tsx
// RunParamsDialog.tsx
export function RunParamsDialog({ schema, onRun, onCancel }: {
    schema: Record<string, string>
    onRun: (params: Record<string, string>) => void
    onCancel: () => void
}) {
    const [values, setValues] = useState<Record<string, string>>({})
    return (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className="bg-neutral-900 rounded-xl p-6 w-96 space-y-4">
                <h3 className="text-sm font-semibold">运行参数</h3>
                {Object.entries(schema).map(([key, type]) => (
                    <div key={key}>
                        <label className="text-xs text-neutral-400 mb-1 block">{key} <span className="text-neutral-600">({type})</span></label>
                        <input value={values[key] ?? ''}
                            onChange={e => setValues(v => ({ ...v, [key]: e.target.value }))}
                            className="w-full px-3 py-2 bg-neutral-800 rounded text-sm" />
                    </div>
                ))}
                <div className="flex gap-2 justify-end">
                    <button onClick={onCancel} className="px-4 py-2 text-sm rounded bg-neutral-700">取消</button>
                    <button onClick={() => onRun(values)} className="px-4 py-2 text-sm rounded bg-blue-600">开始运行</button>
                </div>
            </div>
        </div>
    )
}
```

---

## 7. Wails Bindings

```go
func (a *App) RunWorkflow(id string, params map[string]any) (string, error) {
    // 防止重复运行
    if a.runner.IsRunning(id) {
        return "", fmt.Errorf("运行中，请等待完成")
    }
    return a.runner.Run(context.Background(), id, params)
}

func (a *App) GetRunResult(runID string) (*service.RunResult, error) {
    return a.runSvc.GetResult(runID)
}

func (a *App) GetNodeResult(runID, nodeID string) (*service.NodeResult, error) {
    return a.runSvc.GetNodeResult(runID, nodeID)
}
```

---

## 8. 验收测试

```
1. RunWorkflow() → 节点依次收到 NodeStatusChanged 事件 → 画布颜色依次变化
2. 条件节点路由后，跳过的节点收到 status=skipped
3. 工具节点失败 → RunFailed 事件 → 顶部提示错误
4. GetNodeResult() 返回该节点的 input/output/error/耗时
5. IsRunning() 防止重复运行
6. trigger_manual 有 params schema → RunParamsDialog 弹出 → 用户填写后运行
```
